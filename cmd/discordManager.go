package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/cobra"

	"github.com/bwmarrin/discordgo"

	"github.com/austinpray/ofisu/internal/discord"
	"github.com/austinpray/ofisu/internal/office"
)

var officeFiles string
var discordToken string
var redisURL string

// discordManagerCmd represents the discordController command
var discordManagerCmd = &cobra.Command{
	Use:   "discord-manager",
	Short: "Adds an office to a discord server",
	Run: func(cmd *cobra.Command, args []string) {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
		logger.Println("starting discord-controller")

		// TODO support multiple offices
		if officeFiles == "" {
			fmt.Println("--offices is required")
			os.Exit(1)
		}
		officeFileGlob, err := filepath.Glob(filepath.Join(officeFiles, "*.dot"))
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		offices := map[string]*office.Office{}
		for _, officeFile := range officeFileGlob {
			userOffice, err := office.FromFile(officeFile)
			if err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}
			offices[userOffice.ID] = userOffice
			logger.Println("loaded office: " + officeFile)
		}

		if redisURL == "" {
			fmt.Println("--redis-url is required")
			os.Exit(1)
		}
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		// wait for redis to boot
		opt.MaxRetries = 3

		rdb := redis.NewClient(opt)

		if discordToken == "" {
			fmt.Println("--discord-token is required")
			os.Exit(1)
		}

		discordSession, err := discordgo.New("Bot " + discordToken)
		discordSession.ShouldReconnectOnError = true
		discordSession.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsAllWithoutPrivileged | discordgo.IntentsGuildMembers)

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		cm := discord.ControllerManager{
			RedisClient: rdb,
			Session:     discordSession,
			Offices:     offices,
			Controllers: map[string]*discord.Controller{},
		}

		cm.AttachHandlers()

		{
			err := discordSession.Open()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-sig:
				fmt.Println("\ngot signal")
				ticker.Stop()
				fmt.Println("stopped ticker")
				err := discordSession.Close()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				fmt.Println("closed discord session")
				fmt.Println("\noyasumi~")
				os.Exit(0)
			case <-ticker.C:
				go cm.SyncGuilds()
			}
		}

	},
}

func init() {
	rootCmd.AddCommand(discordManagerCmd)

	discordManagerCmd.Flags().StringVarP(
		&officeFiles,
		"offices",
		"i",
		os.Getenv("OFISU_OFFICES"),
		"office .dot file directory",
	)
	discordManagerCmd.Flags().StringVarP(
		&discordToken,
		"discord-token",
		"t",
		os.Getenv("OFISU_DISCORD_TOKEN"),
		"discord token",
	)
	discordManagerCmd.Flags().StringVarP(
		&redisURL,
		"redis-url",
		"r",
		os.Getenv("OFISU_REDIS_URL"),
		"redis URL like redis://localhost:6379/",
	)
}
