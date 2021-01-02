package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/austinpray/ofisu/internal/office"
	"github.com/bwmarrin/discordgo"
	"github.com/go-redis/redis/v8"
	"golang.org/x/sync/errgroup"
)

/*
ControllerManager adds and deletes controllers for every guild the bot is a part of
TODO: should prolly have some kind of uninstall cleanup
*/
type ControllerManager struct {
	RedisClient *redis.Client
	Session     *discordgo.Session
	TickRate    time.Duration

	Offices map[string]*office.Office

	guildsSynced time.Time

	// TODO visibility
	Controllers   map[string]*Controller
	controllersMu sync.RWMutex
}

// AttachHandlers is the main entrypoint
func (cm *ControllerManager) AttachHandlers() {
	cm.Session.AddHandler(cm.messageRouter)
	cm.Session.AddHandler(cm.guildCreate)
	cm.Session.AddHandler(cm.guildDelete)
}
func (cm *ControllerManager) addMember(s *discordgo.Session, e *discordgo.GuildMemberAdd) {
	cm.controllersMu.RLock()
	defer cm.controllersMu.RUnlock()
	controller = cm.Controllers[e.GuildID]

}

func (cm *ControllerManager) messageRouter(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	content := m.Content

	// Only handle stuff directed at the bot
	prefixWhitelist := []string{
		"o ",
		s.State.User.Mention(),
		"<@!" + s.State.User.ID + ">", // :^/
	}
	hasPrefix := false
	for _, prefix := range prefixWhitelist {
		if strings.HasPrefix(content, prefix) {
			hasPrefix = true
			content = strings.TrimSpace(strings.TrimPrefix(content, prefix))
			break
		}

	}
	if !hasPrefix {
		return
	}

	fmt.Println("passing message to " + m.GuildID)
	cm.controllersMu.RLock()
	controller, exists := cm.Controllers[m.GuildID]
	cm.controllersMu.RUnlock()
	if !exists {
		fmt.Println("unknown guild " + m.GuildID)
		return
	}
	controller.MessageCreate(s, m, content)
}

func (cm *ControllerManager) guildCreate(s *discordgo.Session, m *discordgo.GuildCreate) {
	cm.controllersMu.Lock()
	guild := cm.addGuildController(m.Guild.ID)
	cm.controllersMu.Unlock()
	err := guild.Sync()
	if err != nil {
		fmt.Println(err)
	}
}
func (cm *ControllerManager) guildDelete(s *discordgo.Session, m *discordgo.GuildDelete) {
	cm.controllersMu.Lock()
	defer cm.controllersMu.Unlock()
	cm.removeGuildController(m.Guild.ID)
}

// TODO: should handle errors :^)
func (cm *ControllerManager) addGuildController(guildID string) *Controller {
	_, exists := cm.Controllers[guildID]
	if !exists {
		cm.Controllers[guildID] = &Controller{
			GuildID:     guildID,
			RedisClient: cm.RedisClient,
			Session:     cm.Session,
			Offices:     cm.Offices,
		}
		fmt.Println("added guild: " + guildID)
	}

	return cm.Controllers[guildID]

}

// TODO: should handle errors :^)
func (cm *ControllerManager) removeGuildController(guildID string) {
	_, exists := cm.Controllers[guildID]
	if exists {
		delete(cm.Controllers, guildID)
		fmt.Println("removed guild: " + guildID)
	}
}

func (cm *ControllerManager) SyncGuilds() {
	if time.Since(cm.guildsSynced) >= 12*time.Minute {
		// TODO: hardcoded 100 guild limit
		guilds, err := cm.Session.UserGuilds(100, "", "")
		if err != nil {
			// TODO: error handling
			fmt.Println(err)
			return
		}

		cm.controllersMu.Lock()

		knownGuildSet := map[string]bool{}
		for _, guild := range guilds {
			cm.addGuildController(guild.ID)
			knownGuildSet[guild.ID] = true
		}
		for guildID := range cm.Controllers {
			// check for dangling guilds
			if _, knownGuild := knownGuildSet[guildID]; !knownGuild {
				cm.removeGuildController(guildID)
				continue
			}
		}

		cm.controllersMu.Unlock()

	}

	cm.controllersMu.RLock()

	// TODO: timeout?
	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)
	for _, controller := range cm.Controllers {
		g.Go(controller.Sync)
	}

	cm.controllersMu.RUnlock()

	if err := g.Wait(); err != nil {
		fmt.Println(err)
	}
}
