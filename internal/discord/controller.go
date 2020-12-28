package discord

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/austinpray/ofisu/internal/office"
	"github.com/bwmarrin/discordgo"
	"github.com/go-redis/redis/v8"
)

const channelPrefix = "o"

/*
Controller will reconcile a discord server's state with what is in the
database and the office file definition in an idemptotent manner

To avoid premature optimization, this will just poll at a certain tick rate.
We could probably poll at 60 ticks per second before we run into any issues
under 10 servers.

Since the controller's sync methods are idemptotent, we could totally just
fire the sync methods on whatever discord interaction events that make sense
if latency or rate limits become an issue. For an MVP: YAGNI

Note that this controller does not aquire any locks on the resources it is
modifying. So in a rolling release where two controllers are running
side-by-side, there could be some wasted work. But since each controller is
idempotent, there should not be desyncs.

TODO: a controller should probably aquire a lock on a guild ID to gracefully
support rolling deploys
*/
type Controller struct {
	GuildID     string
	RedisClient *redis.Client
	Session     *discordgo.Session
	Offices     map[string]*office.Office

	state   *RemoteState
	stateMu sync.RWMutex
}

// RemoteState is how discord currently looks
type RemoteState struct {
	Office          *office.Office
	channels        []*discordgo.Channel
	members         []*discordgo.Member
	parentChannelID string
	channelsSynced  time.Time
	usersSynced     time.Time
	membersSynced   time.Time
}

func (s *RemoteState) requestSync() {
	s.channelsSynced = time.Time{}
	s.usersSynced = time.Time{}
}

/*
func (s *RemoteState) requestChannelSync() {
	s.channelsSynced = time.Time{}
}
*/
func (s *RemoteState) requestUserSync() {
	s.usersSynced = time.Time{}
}

const channelSyncInterval = 10 * time.Minute
const userSyncInterval = 5 * time.Minute
const guildMemberSyncInterval = 30 * time.Minute

func (c *Controller) handleErr(err error) error {
	// TODO prolly do something real
	fmt.Println(err)
	return err
}

func (c *Controller) GetInstalledOffice(ctx context.Context) (*office.Office, error) {
	officeID, err := c.RedisClient.Get(ctx, c.installedOfficeKey()).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	office, exists := c.Offices[officeID]
	if !exists {
		return nil, fmt.Errorf("Unknown installed office '%s'", officeID)
	}
	return office, nil
}

// Sync will reconcile the current state with remote
func (c *Controller) Sync() error {
	// TODO should time out
	ctx := context.Background()

	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.state == nil {
		c.state = &RemoteState{}
	}

	// TODO pass in as arg for testability
	now := time.Now()

	if now.Sub(c.state.membersSynced) > guildMemberSyncInterval {
		fmt.Println("doing guild member sync")
		members, err := c.Session.GuildMembers(c.GuildID, "", 1000)
		if err != nil {
			return c.handleErr(err)
		}
		c.state.members = members
		c.state.membersSynced = now
	}

	if now.Sub(c.state.channelsSynced) > channelSyncInterval {
		fmt.Println("doing channel sync")
		installedOffice, err := c.GetInstalledOffice(ctx)
		if err != nil {
			return err
		}
		if c.state.Office != installedOffice {
			c.state.Office = installedOffice
		}

		{
			err := c.syncChannels(ctx, *c.state)
			if err != nil {
				return c.handleErr(err)
			}
		}
		c.state.channelsSynced = now
	}

	if now.Sub(c.state.usersSynced) > userSyncInterval {
		fmt.Println("doing user sync")
		err := c.syncUsers(ctx, *c.state)
		if err != nil {
			return c.handleErr(err)
		}
		c.state.usersSynced = now
	}
	return nil
}

func (c *Controller) isManagedChannel(channelID string) bool {
	for _, channel := range c.state.channels {
		if channel.ID == channelID {
			return channel.ParentID == c.state.parentChannelID
		}
	}

	return false
}

// accepts cries for help
var cmdHelp = regexp.MustCompile(`(?i)^h[ea]+l+p+$`)

// accepts commands like "go to the office" and "enter office"
var cmdEnterOffice = regexp.MustCompile(`(?i)^(?:go|drive|enter)(?:\s+to)?\s+(?:the\s+)?office$`)

// install office command
var cmdInstallOffice = regexp.MustCompile(`(?i)^!install (.+)$`)

// uninstall office command
var cmdUninstallOffice = regexp.MustCompile(`(?i)^!uninstall$`)

// accepts go( to) <room>
var cmdGo = regexp.MustCompile(`(?i)^go(?:\s+to)?\s+(.+)$`)

// accepts (l)ook
var cmdLook = regexp.MustCompile(`(?i)^l(?:ook)?$`)

// MessageCreate handles a discord message create event
// TODO: break me into smaller functions
func (c *Controller) MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate, content string) {
	ctx := context.Background()

	somethingWentWrongSadface := func(err error) {
		fmt.Println(err)
		{
			msg := "something went wrong :("
			_, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s %s", m.Author.Mention(), msg))
			if err != nil {
				fmt.Println("I heard you like errors: " + err.Error())
			}
		}
	}

	if cmdHelp.MatchString(content) {
		_, err := s.ChannelMessageSend(m.ChannelID, `help:
__global commands__
"help" => this message
"go to office" => puts you in the parking lot of the office
"about" => print information about this ofisu install

__ofisu channel commands__
"look" or "l" => look around the room to see where you can go
"go <room>" => will take you to <room>

__admin commands__
"offices" => prints available offices
"!sync" => syncs up discord with the state of your office
"!install <office>" => initializes an office on your server, NOTE: this will completely reset your office
"!uninstall" => removes the office from your server, NOTE: this completely deletes all state
`)
		if err != nil {
			somethingWentWrongSadface(err)
		}

		return
	}

	action := func(msg string) {
		_, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("*%s %s*", m.Author.Mention(), msg))
		if err != nil {
			somethingWentWrongSadface(err)
		}
	}
	ack := func() {
		err := s.MessageReactionAdd(m.ChannelID, m.ID, "👍")
		if err != nil {
			// don't show user, this is just fluff
			fmt.Println(err)
		}
	}
	reply := func(msg string) {
		_, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s %s", m.Author.Mention(), msg))
		if err != nil {
			somethingWentWrongSadface(err)
		}
	}

	// we stateful from here, just lock
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	installedOffice := c.state.Office

	if content == "offices" {
		reply(c.availableOfficesMessage(installedOffice))
		return
	}

	if content == "!sync" {
		c.state.requestSync()
		reply("Will sync everything up in a moment!")
		return
	}

	if cmdEnterOffice.MatchString(content) {
		if installedOffice == nil {
			reply("No office installed, ask an admin to install one.")
			return
		}

		_, err := c.RedisClient.Set(ctx, c.makeUserLocationKey(*m.Author), "parking_lot", 0).Result()
		if err != nil {
			// TODO error handling
			fmt.Println(err)
			reply("Could not move you, try again")
			return
		}

		action("drives up to the office")
		c.state.requestUserSync()

		return
	}

	adminResp, err := c.handleAdminCommands(ctx, m.Author, content)
	if err != nil {
		somethingWentWrongSadface(err)
		return
	}
	if adminResp != "" {
		reply(adminResp)
		return
	}

	// only enable commands in office channels
	if !c.isManagedChannel(m.ChannelID) {
		reply("I don't know how to handle that command. Is it available in this channel? Try asking for help")
		return
	}

	if installedOffice == nil {
		reply("No office installed, ask an admin to install one.")
		return
	}

	// TODO: prolly avoid this call
	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		action("I encountered an error, can you try that again?")
		// TODO: properly handle errors
		fmt.Println(err)
		return
	}

	currentRoomID := strings.TrimPrefix(channel.Name, channelPrefix+"-")
	adjacentRooms := installedOffice.GetAdjacentRooms(currentRoomID)
	moveText := ""
	for _, room := range adjacentRooms {
		moveText += fmt.Sprintf("- %s\n", room.Name)
	}

	if cmdLook.MatchString(content) {
		if len(adjacentRooms) == 0 {
			// logically should not happen
			action("This room has no adjacent rooms (???)")
			return
		}
		reply("From here you can go to:\n" + moveText)

		return

	}

	if matches := cmdGo.FindStringSubmatch(content); matches != nil {
		desired := matches[1]

		candidates := c.state.Office.GetMoveCandidates(currentRoomID, desired)

		if len(candidates) == 0 {
			reply("That destination seems invalid. You can try going to:\n" + moveText)
			return
		}
		if len(candidates) == 1 {
			_, err := c.RedisClient.Set(ctx, c.makeUserLocationKey(*m.Author), candidates[0].ID, 0).Result()
			if err != nil {
				// TODO error handling
				fmt.Println(err)
				reply("Could not move you, try again")
				return
			}
			ack()
			c.state.requestUserSync()
			return
		}

		// ambigious desire
		options := ""
		for _, room := range candidates {
			options += fmt.Sprintf("- %s", room.Name)
		}
		reply("Which one do you want to go to?\n" + options)
		return

	}

	// no matches :(
	reply("I don't know how to handle that command, try asking for help")
}

func (c *Controller) availableOfficesMessage(installedOffice *office.Office) string {
	if len(c.Offices) == 0 {
		return "No offices available"
	}
	msg := "Available offices:\n"
	for _, office := range c.Offices {
		active := ""
		if installedOffice != nil && installedOffice.ID == office.ID {
			active = " (active)"
		}
		msg = msg + fmt.Sprintf("- `%s`%s: %s\n", office.ID, active, office.Name)
	}
	return msg
}

func (c *Controller) handleAdminCommands(ctx context.Context, user *discordgo.User, content string) (string, error) {

	// check if this is an admin command before wasting an API call
	adminCommands := []*regexp.Regexp{
		cmdInstallOffice,
		cmdUninstallOffice,
	}
	isAdminCommand := false
	for _, cmd := range adminCommands {
		if cmd.MatchString(content) {
			isAdminCommand = true
		}
	}
	if !isAdminCommand {
		return "", nil
	}

	isAdmin, err := c.isGuildAdmin(user)
	if err != nil {
		return "", err
	}

	if !isAdmin {
		return "You need to be a server admin to run this command", nil
	}

	// install office command
	if matches := cmdInstallOffice.FindStringSubmatch(content); matches != nil {
		officeID := matches[1]
		newOffice, exists := c.Offices[officeID]
		if !exists {
			msg := fmt.Sprintf("Office with ID '%s' is not available\n", officeID) + c.availableOfficesMessage(c.state.Office)
			return msg, nil
		}
		_, err := c.RedisClient.Set(ctx, c.installedOfficeKey(), officeID, 0).Result()
		if err != nil {
			return "", err
		}
		c.state.requestSync()
		return fmt.Sprintf("'%s' is now installed!", newOffice.Name), nil
	}

	// uninstall office command
	if matches := cmdUninstallOffice.FindStringSubmatch(content); matches != nil {
		_, err := c.RedisClient.Del(ctx, c.installedOfficeKey()).Result()
		if err != nil {
			return "", err
		}
		c.state.requestSync()
		return "office uninstalled!", nil
	}

	return "", nil
}

func (c *Controller) isGuildAdmin(user *discordgo.User) (bool, error) {
	guild, err := c.Session.Guild(c.GuildID)
	if err != nil {
		return false, err
	}

	if user.ID == guild.OwnerID {
		return true, err
	}

	member, err := c.Session.GuildMember(c.GuildID, user.ID)
	if err != nil {
		return false, err
	}
	guildRoles, err := c.Session.GuildRoles(c.GuildID)
	if err != nil {
		return false, err
	}
	guildRoleMap := map[string]*discordgo.Role{}
	for _, role := range guildRoles {
		guildRoleMap[role.ID] = role
	}
	for _, roleID := range member.Roles {
		if (guildRoleMap[roleID].Permissions & discordgo.PermissionAdministrator) != 0 {
			return true, nil
		}
	}

	return false, nil
}

func (c *Controller) channelPermissionOverwrite() discordgo.PermissionOverwrite {
	return discordgo.PermissionOverwrite{
		ID:   c.GuildID,
		Type: "role",
		Deny: discordgo.PermissionViewChannel | discordgo.PermissionCreateInstantInvite,
	}

}

func (c *Controller) getParentChannelID(channels []*discordgo.Channel) (string, error) {
	privateChannelOverwrite := c.channelPermissionOverwrite()

	parentChannelID := ""
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory && channel.Name == "ofisu" {
			parentChannelID = channel.ID
		}
	}
	if parentChannelID == "" {
		_, err := c.Session.GuildChannelCreateComplex(c.GuildID, discordgo.GuildChannelCreateData{
			Name: "ofisu",
			Type: discordgo.ChannelTypeGuildCategory,
			PermissionOverwrites: []*discordgo.PermissionOverwrite{
				&privateChannelOverwrite,
			},
		})
		if err != nil {
			return "", err
		}
	}

	return parentChannelID, nil
}

func (c *Controller) syncChannels(ctx context.Context, state RemoteState) error {

	// grab remote channels
	channels, err := c.Session.GuildChannels(c.GuildID)
	if err != nil {
		return c.handleErr(err)
	}
	c.state.channels = channels

	// set up the parent channel group
	parentChannelID, err := c.getParentChannelID(channels)
	if err != nil {
		return c.handleErr(err)
	}

	if c.state.parentChannelID != parentChannelID {
		c.state.parentChannelID = parentChannelID
	}
	privateChannelOverwrite := c.channelPermissionOverwrite()

	// we keep track of these sets so we don't have to loop over channels twice
	// prolly too fiddly of an optmization but whatever
	existingTextChannels := make(map[string]bool)
	existingVoiceChannels := make(map[string]bool)

	// TODO: each iteration of this loop can be a goroutine
	for _, channel := range channels {
		// TODO: DRY
		if channel.ParentID != parentChannelID {
			continue
		}
		isAllowedType := channel.Type == discordgo.ChannelTypeGuildText || channel.Type == discordgo.ChannelTypeGuildVoice
		if !isAllowedType {
			continue
		}

		// let's check if we are dealing with a dangling channel
		var room office.Room
		var shouldExist bool
		if state.Office == nil {
			// uninstall
			shouldExist = false
		} else {
			room, shouldExist = state.Office.Rooms[strings.TrimPrefix(channel.Name, channelPrefix+"-")]
		}
		if !shouldExist {
			_, err := c.Session.ChannelDelete(channel.ID)
			if err != nil {
				return err
			}
			continue
		}
		// handle dangling voice channels
		if !room.VoiceEnabled && channel.Type == discordgo.ChannelTypeGuildVoice {
			_, err := c.Session.ChannelDelete(channel.ID)
			if err != nil {
				return err
			}
			continue
		}

		// apply topic
		topic := ""
		if room.Name != room.ID {
			topic = room.Name
		}
		if channel.Topic != topic {
			_, err := c.Session.ChannelEditComplex(channel.ID, &discordgo.ChannelEdit{
				Topic: topic,
			})
			if err != nil {
				return err
			}
		}

		// ensure permissions are correct
		for _, permissionOverwrite := range channel.PermissionOverwrites {
			if permissionOverwrite.ID != privateChannelOverwrite.ID {
				continue
			}
			if permissionOverwrite.Deny != privateChannelOverwrite.Deny {
				err := c.Session.ChannelPermissionSet(
					channel.ID,
					privateChannelOverwrite.ID,
					privateChannelOverwrite.Type,
					0,
					privateChannelOverwrite.Deny,
				)
				if err != nil {
					return err
				}
			}
		}

		if channel.Type == discordgo.ChannelTypeGuildVoice {
			existingVoiceChannels[channel.Name] = true
		} else {
			existingTextChannels[channel.Name] = true

		}
	}

	if state.Office == nil {
		// no office installed
		return nil
	}

	// TODO: each iteration of this loop can be a goroutine
	for roomName, room := range state.Office.Rooms {
		channelName := channelPrefix + "-" + roomName
		textChannelExists := existingTextChannels[channelName]
		if !textChannelExists {
			topic := ""
			if room.Name != roomName {
				topic = room.Name
			}
			_, err := c.Session.GuildChannelCreateComplex(c.GuildID, discordgo.GuildChannelCreateData{
				Name:     channelName,
				Type:     discordgo.ChannelTypeGuildText,
				Topic:    topic,
				ParentID: parentChannelID,
				PermissionOverwrites: []*discordgo.PermissionOverwrite{
					&privateChannelOverwrite,
				},
			})
			if err != nil {
				return err
			}
		}
		voiceChannelExists := existingVoiceChannels[channelName]
		if !voiceChannelExists && room.VoiceEnabled {
			topic := ""
			if room.Name != roomName {
				topic = room.Name
			}
			_, err := c.Session.GuildChannelCreateComplex(c.GuildID, discordgo.GuildChannelCreateData{
				Name:     channelName,
				Type:     discordgo.ChannelTypeGuildVoice,
				Topic:    topic,
				ParentID: parentChannelID,
				PermissionOverwrites: []*discordgo.PermissionOverwrite{
					&privateChannelOverwrite,
				},
			})
			if err != nil {
				return err
			}
		}
	}

	{
		// grab remote channels
		channels, err := c.Session.GuildChannels(c.GuildID)
		if err != nil {
			return c.handleErr(err)
		}
		c.state.channels = channels
	}

	return nil
}

func (c *Controller) syncUsers(ctx context.Context, state RemoteState) error {
	if state.Office == nil {
		return nil
	}
	channels := state.channels
	members := state.members

	userLocationKeys := []string{}
	users := map[string]*discordgo.User{}
	for _, member := range members {
		userLocationKeys = append(userLocationKeys, c.makeUserLocationKey(*member.User))
		users[member.User.ID] = member.User
	}

	results, err := c.RedisClient.MGet(ctx, userLocationKeys...).Result()
	if err != nil {
		return err
	}

	userDesiredRooms := make(map[string]string)
	resultRooms := make([]string, len(results))
	for i, result := range results {
		if result == nil {
			resultRooms[i] = ""
			continue
		}
		resultRooms[i] = result.(string)
	}
	for i, room := range resultRooms {
		if room == "" {
			continue
		}

		userDesiredRooms[members[i].User.ID] = room
	}

	for _, channel := range channels {
		if !c.isManagedChannel(channel.ID) {
			continue
		}
		currentRoomID := strings.TrimPrefix(channel.Name, channelPrefix+"-")
		for userID, desiredRoomID := range userDesiredRooms {
			currentRoom, exists := state.Office.Rooms[currentRoomID]
			if !exists {
				return fmt.Errorf("currentRoom '%s' does not exist", currentRoomID)
			}
			desiredRoom, exists := state.Office.Rooms[desiredRoomID]
			if !exists {
				return fmt.Errorf("desiredRoom '%s' does not exist", desiredRoomID)
			}
			user, exists := users[userID]
			if !exists {
				return fmt.Errorf("user '%s' does not exist", userID)
			}

			userPerms, err := c.Session.UserChannelPermissions(userID, channel.ID)
			if err != nil {
				return fmt.Errorf("cannot get user perms: %v", err)
			}

			hasAccessToChannel := (discordgo.PermissionViewChannel & userPerms) != 0

			if currentRoomID != desiredRoomID && hasAccessToChannel {
				msg := fmt.Sprintf(
					"*%s left %s heading towards %s*",
					user.Mention(),
					currentRoom.Name,
					desiredRoom.Name,
				)
				if channel.Type == discordgo.ChannelTypeGuildText {
					_, err := c.Session.ChannelMessageSend(channel.ID, msg)
					if err != nil {
						return fmt.Errorf("failed to send leave message: %v", err)
					}
				}

				err := c.Session.ChannelPermissionDelete(channel.ID, userID)
				if err != nil {
					return fmt.Errorf("cannot delete perms: %v", err)
				}
				continue
			}
			if currentRoomID == desiredRoomID && !hasAccessToChannel {
				err := c.Session.ChannelPermissionSet(channel.ID, userID, "member", discordgo.PermissionViewChannel, 0)
				if err != nil {
					return fmt.Errorf("cannot add perms: %v", err)
				}
				msg := fmt.Sprintf(
					"*%s entered %s*",
					user.Mention(),
					desiredRoom.Name,
				)
				if channel.Type == discordgo.ChannelTypeGuildText {
					_, err := c.Session.ChannelMessageSend(channel.ID, msg)
					if err != nil {
						return fmt.Errorf("failed to send leave message: %v", err)
					}
				}
				continue
			}
		}
	}

	return nil
}

// TODO: take all these make...Key functions and turn them into a key manager

func (c *Controller) makeKeyBase() string {
	// warning: changing this key structure will require a data migration
	parts := []string{
		"ofisu",
		"discord",
		"guilds",
		c.GuildID,
	}
	return strings.Join(parts, "/")
}

func (c *Controller) installedOfficeKey() string {
	return c.makeKeyBase() + "/installed_office"
}

func (c *Controller) makeOfficeKey() string {
	// warning: changing this key structure will require a data migration
	parts := []string{
		c.makeKeyBase(),
		"office",
		c.state.Office.ID,
	}
	return strings.Join(parts, "/")
}

func (c *Controller) makeUserKey(user discordgo.User) string {
	// warning: changing this key structure will require a data migration
	parts := []string{
		c.makeOfficeKey(),
		"users",
		user.ID,
	}
	return strings.Join(parts, "/")
}

func (c *Controller) makeUserLocationKey(user discordgo.User) string {
	// warning: changes require data migration
	return c.makeUserKey(user) + "/location"
}
