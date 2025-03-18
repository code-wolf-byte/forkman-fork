package economy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/code-wolf-byte/forkman/internal/database"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// Constants matching your module pattern
const (
	name        = "Economy"
	description = "Points & achievements system"
)

var (
	ErrModuleAlreadyDisabled = errors.New("module is already disabled")
	ErrModuleAlreadyEnabled  = errors.New("module is already enabled")
)

// Economy is the main struct for your economy module
type Economy struct {
	guildName      string
	guildSnowflake string
	appId          string
	session        *discordgo.Session
	repo           *Repository
	log            *zerolog.Logger
}

// New returns an instance of the economy module
func New(
	guildName string,
	guildSnowflake string,
	appId string,
	session *discordgo.Session,
	db *gorm.DB,
	log *zerolog.Logger,
) *Economy {
	l := log.With().
		Str("module", name).
		Str("guild_name", guildName).
		Str("guild_snowflake", guildSnowflake).
		Logger()

	return &Economy{
		guildName:      guildName,
		guildSnowflake: guildSnowflake,
		appId:          appId,
		session:        session,
		repo:           NewRepository(db),
		log:            &l,
	}
}

// Load is called when a guild first becomes available or on reconnect
func (e *Economy) Load() error {
	// 1) Check DB for existing module row or create one if none
	mod, err := e.repo.ReadModule(e.guildSnowflake)
	if err == gorm.ErrRecordNotFound {
		e.log.Debug().Msg("economy module not found, creating...")

		// default config
		cfgJson, _ := json.Marshal(struct{}{}) // or define a real EconomyConfig if you prefer

		// default command states
		cmdMap := make(map[string]bool)
		for _, cmd := range commands {
			cmdMap[cmd.Name] = true
		}
		cmdJson, _ := json.Marshal(cmdMap)

		// create row
		insert := &database.Module{
			GuildSnowflake: e.guildSnowflake,
			Name:           name,
			Description:    description,
			Enabled:        true,
			Config:         cfgJson,
			Commands:       cmdJson,
		}
		if mod, err = e.repo.CreateModule(insert); err != nil {
			return fmt.Errorf("unable to create economy module: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("unable to read economy module from DB: %w", err)
	}

	// 2) Seed default events like "DAILY" if they don't exist
	if err := e.repo.SeedDefaultEvents(); err != nil {
		return fmt.Errorf("failed to seed economy events: %w", err)
	}

	// 3) If the module is disabled, skip command registration
	if !mod.Enabled {
		e.log.Debug().Msg("economy module disabled, skipping load")
		return nil
	}

	// 4) Unmarshal command map to see which slash commands are toggled on
	var cmds map[string]bool
	if err := json.Unmarshal([]byte(mod.Commands), &cmds); err != nil {
		return fmt.Errorf("critical error unmarshalling command map: %w", err)
	}

	// 5) Register slash commands for those that are enabled
	updated := false
	for _, cmd := range commands {
		if _, ok := cmds[cmd.Name]; !ok {
			// new command not in DB yet
			cmds[cmd.Name] = true
			updated = true
		}
	}
	if updated {
		newCmdJson, _ := json.Marshal(cmds)
		mod.Commands = newCmdJson
		_, err = e.repo.UpdateModule(mod)
		if err != nil {
			return fmt.Errorf("unable to update economy module commands: %w", err)
		}
	}

	// Actually register slash commands on Discord
	for _, cmd := range commands {
		if !cmds[cmd.Name] {
			e.log.Debug().Str("command", cmd.Name).Msg("command disabled, skipping")
			continue
		}
		_, err := e.session.ApplicationCommandCreate(e.appId, e.guildSnowflake, cmd)
		if err != nil {
			e.log.Error().Err(err).Str("command", cmd.Name).Msg("error registering command")
		}
	}

	e.log.Debug().Msgf("economy module loaded for guild %s", e.guildName)
	return nil
}

// Enable sets the economy module as enabled in DB and registers commands
func (e *Economy) Enable() error {
	mod, err := e.repo.ReadModule(e.guildSnowflake)
	if err != nil {
		return err
	}
	if mod.Enabled {
		return ErrModuleAlreadyEnabled
	}
	mod.Enabled = true
	if _, err := e.repo.UpdateModule(mod); err != nil {
		return err
	}

	// register commands
	var cmds map[string]bool
	if err := json.Unmarshal([]byte(mod.Commands), &cmds); err != nil {
		return fmt.Errorf("unmarshal commands: %w", err)
	}
	for _, cmd := range commands {
		if !cmds[cmd.Name] {
			continue
		}
		if _, err := e.session.ApplicationCommandCreate(e.appId, e.guildSnowflake, cmd); err != nil {
			e.log.Error().Err(err).Str("cmd", cmd.Name).Msg("error registering command")
		}
	}

	e.log.Info().Msg("economy module enabled")
	return nil
}

// Disable sets the economy module as disabled in DB and removes commands from Discord
func (e *Economy) Disable() error {
	mod, err := e.repo.ReadModule(e.guildSnowflake)
	if err != nil {
		return err
	}
	if !mod.Enabled {
		return ErrModuleAlreadyDisabled
	}
	mod.Enabled = false

	if _, err := e.repo.UpdateModule(mod); err != nil {
		return err
	}

	// remove relevant commands from Discord
	remote, err := e.session.ApplicationCommands(e.appId, e.guildSnowflake)
	if err != nil {
		return fmt.Errorf("unable to fetch remote commands: %w", err)
	}
	for _, c := range remote {
		// Optionally, check if c.Name belongs to this module’s known commands
		for _, known := range commands {
			if c.Name == known.Name {
				e.session.ApplicationCommandDelete(e.appId, e.guildSnowflake, c.ID)
			}
		}
	}

	e.log.Info().Msg("economy module disabled")
	return nil
}

// Status returns true if the module is enabled, otherwise false
func (e *Economy) Status() (bool, error) {
	mod, err := e.repo.ReadModule(e.guildSnowflake)
	if err != nil {
		return false, err
	}
	return mod.Enabled, nil
}

// OnInteractionCreate processes slash commands or message components
func (e *Economy) OnInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Only handle if module is enabled
	mod, err := e.repo.ReadModule(i.GuildID)
	if err != nil || !mod.Enabled {
		return
	}

	if i.Type == discordgo.InteractionApplicationCommand {
		e.handleCommand(s, i)
	}
	// If you want buttons or modals, do those too
}

// handleCommand routes each slash command
func (e *Economy) handleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cmdName := i.ApplicationCommandData().Name
	switch cmdName {
	case "daily":
		e.handleDailyCommand(s, i)
	case "leaderboard":
		e.handleLeaderboardCommand(s, i)
	case "give":
		e.handleGiveCommand(s, i)
	case "giveall":
		e.handleGiveAllCommand(s, i)
	default:
		// no-op
	}
}

func (e *Economy) handleDailyCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID := i.Member.User.ID
	guildID := i.GuildID

	// Attempt to log "DAILY" event
	err := e.repo.LogEvent(guildID, userID, "DAILY")
	if err != nil {
		// If MaxOccurrence is reached or some other error
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Daily Reward Error",
						Description: "You have already claimed your daily reward or an error occurred.",
						Color:       0xFF0000, // red
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Otherwise success: show how many total points the user has now
	total, _ := e.repo.GetUserPoints(guildID, userID)
	resp := fmt.Sprintf("You claimed your daily reward!\nYour new total is %d points.", total)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       "Daily Reward Claimed!",
					Description: resp,
					Color:       0x00FF00, // green
				},
			},
		},
	})
}

func (e *Economy) handleLeaderboardCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID := i.GuildID
	top, err := e.repo.GetTopUsers(guildID, 10) // top 10
	if err != nil || len(top) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Leaderboard",
						Description: "No leaderboard data found!",
						Color:       0xFF0000, // red
					},
				},
			},
		})
		return
	}

	// Build lines
	lines := []string{}
	for idx, row := range top {
		line := fmt.Sprintf("**%d.** <@%s> — %d points", idx+1, row.UserSnowflake, row.Points)
		lines = append(lines, line)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Top 10 Leaderboard",
		Description: strings.Join(lines, "\n"),
		Color:       0x00FF00, // green
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (e *Economy) handleGiveCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	opts := data.Options
	if len(opts) < 2 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Usage Error",
						Description: "Usage: /give user:@User event:<event_key>",
						Color:       0xFF0000,
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	userValue := opts[0].UserValue(s)
	eventKey := opts[1].StringValue()

	err := e.repo.LogEvent(i.GuildID, userValue.ID, eventKey)
	if err != nil {
		embed := &discordgo.MessageEmbed{
			Title:       "Failed to Award Event",
			Description: fmt.Sprintf("Could not award event [%s] to <@%s>: %v", eventKey, userValue.ID, err),
			Color:       0xFF0000,
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
				Flags:  discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	total, _ := e.repo.GetUserPoints(i.GuildID, userValue.ID)
	resp := fmt.Sprintf("Gave event [%s] to <@%s>.\nThey now have **%d points**.", eventKey, userValue.ID, total)

	embed := &discordgo.MessageEmbed{
		Title:       "Event Awarded",
		Description: resp,
		Color:       0x00FF00,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral, // or remove if you want public
		},
	})
}

func (e *Economy) handleGiveAllCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	opts := data.Options
	if len(opts) < 1 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Usage Error",
						Description: "Usage: /giveall event:<event_key>",
						Color:       0xFF0000,
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	eventKey := opts[0].StringValue()

	guildID := i.GuildID

	// Retrieve up to 1000 members (the partial approach)
	members, err := s.GuildMembers(guildID, "", 1000)
	if err != nil {
		embed := &discordgo.MessageEmbed{
			Title:       "Error Retrieving Guild Members",
			Description: "Could not fetch guild members. Try again later?",
			Color:       0xFF0000,
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
				Flags:  discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	count := 0
	for _, mem := range members {
		if mem.User.Bot {
			continue
		}
		err := e.repo.LogEvent(guildID, mem.User.ID, eventKey)
		if err == nil {
			count++
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Mass Award Complete",
		Description: fmt.Sprintf("Event [%s] awarded to %d members!", eventKey, count),
		Color:       0x00FF00,
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}
