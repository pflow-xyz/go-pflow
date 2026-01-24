package metamodel

// EventSourcingConfig represents event sourcing configuration.
type EventSourcingConfig struct {
	Snapshots *SnapshotConfig  `json:"snapshots,omitempty"`
	Retention *RetentionConfig `json:"retention,omitempty"`
}

// SnapshotConfig controls automatic snapshot creation.
type SnapshotConfig struct {
	Enabled   bool `json:"enabled"`
	Frequency int  `json:"frequency"` // Every N events
}

// RetentionConfig controls event and snapshot retention.
type RetentionConfig struct {
	Events    string `json:"events"`    // e.g., "90d"
	Snapshots string `json:"snapshots"` // e.g., "1y"
}

// Debug represents debug configuration for development and testing.
type Debug struct {
	Enabled bool `json:"enabled"`
	Eval    bool `json:"eval,omitempty"` // Enable eval endpoint
}

// SLAConfig represents workflow-level SLA configuration.
type SLAConfig struct {
	Default    string            `json:"default,omitempty"`    // Default SLA duration
	ByPriority map[string]string `json:"byPriority,omitempty"` // SLA by priority level
	WarningAt  float64           `json:"warningAt,omitempty"`  // Percentage for warning status
	CriticalAt float64           `json:"criticalAt,omitempty"` // Percentage for critical status
	OnBreach   string            `json:"onBreach,omitempty"`   // Action on breach: "alert", "log", "webhook"
}

// PredictionConfig represents ODE-based prediction configuration.
type PredictionConfig struct {
	Enabled   bool    `json:"enabled"`
	TimeHours float64 `json:"timeHours,omitempty"` // Simulation duration in hours
	RateScale float64 `json:"rateScale,omitempty"` // Rate scaling factor
}

// GraphQLConfig represents GraphQL API configuration.
type GraphQLConfig struct {
	Enabled    bool   `json:"enabled"`
	Path       string `json:"path,omitempty"`       // Endpoint path
	Playground bool   `json:"playground,omitempty"` // Enable Playground
}

// BlobstoreConfig represents blobstore configuration for event attachments.
type BlobstoreConfig struct {
	Enabled      bool     `json:"enabled"`
	MaxSize      int64    `json:"maxSize,omitempty"`      // Maximum blob size in bytes
	AllowedTypes []string `json:"allowedTypes,omitempty"` // Allowed content types
}

// WalletConfig configures the debug wallet mockup for testing.
type WalletConfig struct {
	Enabled      bool            `json:"enabled"`
	Accounts     []WalletAccount `json:"accounts,omitempty"`
	BalanceField string          `json:"balanceField,omitempty"` // State field for balances
	ShowInNav    bool            `json:"showInNav,omitempty"`
	AutoConnect  bool            `json:"autoConnect,omitempty"`
}

// WalletAccount represents a pre-configured test wallet account.
type WalletAccount struct {
	Address        string   `json:"address"`
	Name           string   `json:"name,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	InitialBalance string   `json:"initialBalance,omitempty"`
}

// CommentsConfig represents comments/notes configuration.
type CommentsConfig struct {
	Enabled    bool     `json:"enabled"`
	Roles      []string `json:"roles,omitempty"`      // Roles that can comment
	Moderation bool     `json:"moderation,omitempty"` // Require moderation
	MaxLength  int      `json:"maxLength,omitempty"`  // Maximum comment length
}

// TagsConfig represents tags/labels configuration.
type TagsConfig struct {
	Enabled    bool     `json:"enabled"`
	Predefined []string `json:"predefined,omitempty"` // Predefined tag options
	FreeForm   bool     `json:"freeForm,omitempty"`   // Allow free-form tags
	MaxTags    int      `json:"maxTags,omitempty"`    // Maximum tags per instance
	Colors     bool     `json:"colors,omitempty"`     // Enable tag colors
}

// ActivityConfig represents activity feed configuration.
type ActivityConfig struct {
	Enabled       bool     `json:"enabled"`
	IncludeEvents []string `json:"includeEvents,omitempty"` // Event types to include
	ExcludeEvents []string `json:"excludeEvents,omitempty"` // Event types to exclude
	MaxItems      int      `json:"maxItems,omitempty"`      // Maximum items in feed
}

// FavoritesConfig represents favorites/watchlist configuration.
type FavoritesConfig struct {
	Enabled      bool `json:"enabled"`
	Notify       bool `json:"notify,omitempty"`       // Notify on changes
	MaxFavorites int  `json:"maxFavorites,omitempty"` // Maximum favorites per user
}

// ExportConfig represents data export configuration.
type ExportConfig struct {
	Enabled bool     `json:"enabled"`
	Formats []string `json:"formats,omitempty"` // Allowed formats: csv, json, xlsx
	MaxRows int      `json:"maxRows,omitempty"` // Maximum rows per export
	Roles   []string `json:"roles,omitempty"`   // Roles that can export
}

// SoftDeleteConfig represents soft delete configuration.
type SoftDeleteConfig struct {
	Enabled       bool     `json:"enabled"`
	RetentionDays int      `json:"retentionDays,omitempty"` // Days to retain before permanent delete
	RestoreRoles  []string `json:"restoreRoles,omitempty"`  // Roles that can restore
}

// StatusConfig represents human-readable status labels for workflow states.
type StatusConfig struct {
	Places  map[string]string `json:"places,omitempty"` // Place ID to label mapping
	Default string            `json:"default,omitempty"` // Default label
}
