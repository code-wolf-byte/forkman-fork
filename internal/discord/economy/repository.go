package economy

import (
	"fmt"

	"github.com/code-wolf-byte/forkman/internal/database"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateModule, ReadModule, UpdateModule: same pattern as your other modules

func (r *Repository) CreateModule(mod *database.Module) (*database.Module, error) {
	result := r.db.Create(mod)
	return mod, result.Error
}

func (r *Repository) ReadModule(guildSnowflake string) (*database.Module, error) {
	m := &database.Module{}
	result := r.db.
		First(m, "name = ? AND guild_snowflake = ?", name, guildSnowflake)
	return m, result.Error
}

func (r *Repository) UpdateModule(mod *database.Module) (*database.Module, error) {
	m := &database.Module{}
	result := r.db.
		First(m, "name = ? AND guild_snowflake = ?", name, mod.GuildSnowflake)
	if result.Error != nil {
		return nil, result.Error
	}
	m.Enabled = mod.Enabled
	m.Config = mod.Config
	m.Commands = mod.Commands

	err := r.db.Save(m).Error
	return m, err
}

// SeedDefaultEvents inserts any needed EconomyEventDefinition records if they don’t exist.
// You could keep them in a slice or load from config.
func (r *Repository) SeedDefaultEvents() error {
	defaults := []database.EconomyEventDefinition{
		{Key: "DAILY", Name: "Daily Reward", Points: 85, MaxOccurrence: 1},
		{Key: "JOIN_SERVER", Name: "Join the server", Points: 213, MaxOccurrence: 1},
		{Key: "FIRST_MESSAGE", Name: "Send first message", Points: 425, MaxOccurrence: 1},
		{Key: "BIRTHDAY_SET", Name: "Set your birthday", Points: 595, MaxOccurrence: 1},
		{Key: "RESPOND_DAILY_ENGAGEMENT", Name: "Respond to Daily Engagement", Points: 850, MaxOccurrence: 1},
		{Key: "VERIFY_ACCOUNT", Name: "Verify Account", Points: 850, MaxOccurrence: 1},
		{Key: "BOOST_SERVER", Name: "Boost server", Points: 3570, MaxOccurrence: 1},
		{Key: "SUBMIT_DEPOSIT", Name: "Submit enrollment deposit", Points: 34000, MaxOccurrence: 1},
		{Key: "SOCIAL_MEDIA_ENGAGEMENT", Name: "Social Media Engagement", Points: 850, MaxOccurrence: 1},
		// etc. add all your events here
	}

	for _, evt := range defaults {
		// Upsert approach
		var existing database.EconomyEventDefinition
		err := r.db.Where("key = ?", evt.Key).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			// Insert
			if err := r.db.Create(&evt).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

// LogEvent attempts to “trigger” a given eventKey for a user, awarding points if allowed
func (r *Repository) LogEvent(guildID, userID, eventKey string) error {
	// 1) Lookup the event definition
	var def database.EconomyEventDefinition
	if err := r.db.Where("key = ?", eventKey).First(&def).Error; err != nil {
		return fmt.Errorf("unknown event key [%s]: %w", eventKey, err)
	}

	// 2) Count how many times they've triggered it
	var count int64
	r.db.Model(&database.UserEventLog{}).
		Where("economy_event_definition_id = ? AND guild_snowflake = ? AND user_snowflake = ?",
			def.ID, guildID, userID).
		Count(&count)

	// 3) If they've already hit the max, do nothing
	if def.MaxOccurrence > 0 && uint(count) >= def.MaxOccurrence {
		return fmt.Errorf("MaxOccurrence for event [%s] reached", eventKey)
	}

	// 4) Insert a log row
	logRow := database.UserEventLog{
		EconomyEventDefinitionID: def.ID,
		GuildSnowflake:           guildID,
		UserSnowflake:            userID,
	}
	if err := r.db.Create(&logRow).Error; err != nil {
		return err
	}

	// 5) Upsert user’s points in UserEconomy
	var ue database.UserEconomy
	res := r.db.Where("guild_snowflake = ? AND user_snowflake = ?", guildID, userID).First(&ue)
	if res.Error != nil {
		if res.Error == gorm.ErrRecordNotFound {
			// Create brand new row
			ue = database.UserEconomy{
				GuildSnowflake: guildID,
				UserSnowflake:  userID,
				Points:         def.Points,
			}
			return r.db.Create(&ue).Error
		}
		return res.Error
	}

	// Otherwise update existing
	ue.Points += def.Points
	return r.db.Save(&ue).Error
}

// GetUserPoints returns a user’s total points
func (r *Repository) GetUserPoints(guildID, userID string) (uint, error) {
	var ue database.UserEconomy
	err := r.db.Where("guild_snowflake = ? AND user_snowflake = ?", guildID, userID).
		First(&ue).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	return ue.Points, nil
}

// GetTopUsers returns the top N users by points
func (r *Repository) GetTopUsers(guildID string, limit int) ([]database.UserEconomy, error) {
	var users []database.UserEconomy
	err := r.db.
		Where("guild_snowflake = ?", guildID).
		Order("points DESC").
		Limit(limit).
		Find(&users).Error
	return users, err
}

func (r *Repository) GetServerEventNames(guildID string) ([]string, error) {
	var eventNames []string
	err := r.db.Model(&database.EconomyEventDefinition{}).
		Select("name").
		Where("guild_snowflake = ?", guildID).
		Pluck("name", &eventNames).Error
	return eventNames, err
}
