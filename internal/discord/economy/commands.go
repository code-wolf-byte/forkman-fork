package economy

import "github.com/bwmarrin/discordgo"

// commands is the list of slash commands your Economy module will register
var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "daily",
		Description: "Claim your daily reward",
	},
	{
		Name:        "leaderboard",
		Description: "Show the top server members by points",
	},
	{
		Name:        "give",
		Description: "Give an event-based reward to a user",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "Select a user to reward",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "event",
				Description: "Which event key to award? (e.g. SUBMIT_DEPOSIT, BOOST_SERVER, etc.)",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{
						Name:  "Submit Deposit",
						Value: "SUBMIT_DEPOSIT",
					},
					{
						Name:  "Boost Server",
						Value: "BOOST_SERVER",
					},
				},
			},
		},
	},
	{
		Name:        "giveall",
		Description: "Give an event-based reward to everyone in the server",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "event",
				Description: "Which event key to award?",
				Required:    true,
			},
		},
	},
}
