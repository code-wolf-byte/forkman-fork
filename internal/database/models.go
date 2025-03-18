package database

import (
	"time"

	uuid "github.com/satori/go.uuid"
	"gorm.io/datatypes"
)

type User struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:(gen_random_uuid())"`
	DiscordSnowflake string    `gorm:"uniqueIndex"`
	DiscordUsername  string
	DiscordAvatarURL string
	DiscordEmail     string `gorm:"uniqueIndex"`
	LastLogin        time.Time
	CreatedAt        time.Time // Managed by GORM
	UpdatedAt        time.Time // Managed by GORM
}

type Guild struct {
	Snowflake  string `gorm:"primaryKey;unique"`
	Name       string
	IconUrl    string
	OwnerID    string
	Admins     []User    `gorm:"many2many:guild_admins;"`
	AdminRoles []string  `gorm:"type:text[]"`
	Modules    []Module  `gorm:"foreignKey:GuildSnowflake;references:Snowflake;constraint:OnDelete:CASCADE"`
	Emails     []Email   `gorm:"foreignKey:GuildSnowflake;references:Snowflake;constraint:OnDelete:CASCADE"`
	CreatedAt  time.Time // Managed by GORM
	UpdatedAt  time.Time // Managed by GORM
}

type Module struct {
	ID             uint   `gorm:"primarykey;autoIncrement"`
	GuildSnowflake string `gorm:"index"`
	Name           string
	Description    string
	Enabled        bool `gorm:"default:false"`
	Config         datatypes.JSON
	Commands       datatypes.JSON
	CreatedAt      time.Time // Managed by GORM
	UpdatedAt      time.Time // Managed by GORM
}

type Email struct {
	ID             uint   `gorm:"primarykey;autoIncrement"`
	GuildSnowflake string `gorm:"index"`
	UserSnowflake  string
	Address        string
	Code           string
	IsVerified     bool
	CreatedAt      time.Time // Managed by GORM
	UpdatedAt      time.Time // Managed by GORM
}

// EconomyEventDefinition is your master list of recognized events/achievements.
type EconomyEventDefinition struct {
	ID   uint   `gorm:"primaryKey"`
	Key  string `gorm:"uniqueIndex"`
	Name string

	Points        uint
	MaxOccurrence uint

	UserEventLogs []UserEventLog `gorm:"foreignKey:EconomyEventDefinitionID;references:ID"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserEventLog logs each time a user triggers a particular event.
type UserEventLog struct {
	ID uint `gorm:"primaryKey"`

	EconomyEventDefinitionID uint
	EconomyEventDefinition   *EconomyEventDefinition `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	GuildSnowflake string
	UserSnowflake  string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserEconomy tracks the user’s overall points in the guild.
type UserEconomy struct {
	ID             uint   `gorm:"primaryKey"`
	GuildSnowflake string `gorm:"index"`
	UserSnowflake  string `gorm:"index"`
	Points         uint

	CreatedAt time.Time
	UpdatedAt time.Time
}
