package metamodel

// Timer represents a scheduled or delayed transition trigger.
type Timer struct {
	ID         string `json:"id,omitempty"`
	Transition string `json:"transition"`          // Transition to fire
	After      string `json:"after,omitempty"`     // Duration after entering state
	Cron       string `json:"cron,omitempty"`      // Cron expression for scheduled firing
	From       string `json:"from,omitempty"`      // Place that triggers the timer
	Condition  string `json:"condition,omitempty"` // Optional condition expression
	Repeat     bool   `json:"repeat,omitempty"`    // Whether to repeat (for cron timers)
}

// Notification represents a notification triggered by state changes.
type Notification struct {
	ID        string            `json:"id,omitempty"`
	On        string            `json:"on"`                  // Transition or place that triggers
	Channel   string            `json:"channel"`             // email, sms, slack, webhook, in_app
	To        string            `json:"to,omitempty"`        // Recipient expression
	Template  string            `json:"template,omitempty"`  // Template ID or inline template
	Subject   string            `json:"subject,omitempty"`   // Subject line (for email)
	Webhook   string            `json:"webhook,omitempty"`   // Webhook URL
	Condition string            `json:"condition,omitempty"` // Optional condition expression
	Data      map[string]string `json:"data,omitempty"`      // Additional data
}

// Relationship represents a link between workflow instances.
type Relationship struct {
	Name       string `json:"name"`                 // Relationship name
	Type       string `json:"type"`                 // hasMany, hasOne, belongsTo
	Target     string `json:"target"`               // Target model/workflow name
	ForeignKey string `json:"foreignKey,omitempty"` // Foreign key field name
	Cascade    string `json:"cascade,omitempty"`    // Cascade behavior: delete, nullify, restrict
}

// ComputedField represents a derived value from state.
type ComputedField struct {
	Name        string   `json:"name"`
	Type        string   `json:"type,omitempty"`      // Result type: string, number, boolean, array
	Expr        string   `json:"expr"`                // Expression to compute value
	DependsOn   []string `json:"dependsOn,omitempty"` // Fields this depends on (for caching)
	Persisted   bool     `json:"persisted,omitempty"` // Whether to store computed value
	Description string   `json:"description,omitempty"`
}

// Index represents a searchable index on workflow data.
type Index struct {
	Name   string   `json:"name,omitempty"`
	Fields []string `json:"fields"`
	Type   string   `json:"type,omitempty"` // btree, fulltext, hash
	Unique bool     `json:"unique,omitempty"`
}

// ApprovalChain represents a multi-step approval workflow.
type ApprovalChain struct {
	Levels        []ApprovalLevel `json:"levels"`
	EscalateAfter string          `json:"escalateAfter,omitempty"` // Duration before escalation
	OnReject      string          `json:"onReject,omitempty"`      // Transition to fire on rejection
	OnApprove     string          `json:"onApprove,omitempty"`     // Transition to fire on approval
	Parallel      bool            `json:"parallel,omitempty"`      // Whether levels can approve in parallel
}

// ApprovalLevel represents a single level in an approval chain.
type ApprovalLevel struct {
	Role       string `json:"role,omitempty"`
	User       string `json:"user,omitempty"`       // Specific user expression
	Condition  string `json:"condition,omitempty"`  // Condition for this level to apply
	Required   int    `json:"required,omitempty"`   // Number of approvals required
	Transition string `json:"transition,omitempty"` // Custom transition for this level
}

// Template represents a pre-configured starting state.
type Template struct {
	ID          string         `json:"id"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Data        map[string]any `json:"data,omitempty"`    // Pre-filled data
	Roles       []string       `json:"roles,omitempty"`   // Roles that can use this template
	Default     bool           `json:"default,omitempty"` // Whether this is the default template
}

// BatchConfig represents batch operations configuration.
type BatchConfig struct {
	Enabled     bool     `json:"enabled"`
	Transitions []string `json:"transitions,omitempty"` // Transitions allowed in batch
	MaxSize     int      `json:"maxSize,omitempty"`     // Maximum batch size
}

// InboundWebhook represents an external webhook endpoint.
type InboundWebhook struct {
	ID         string            `json:"id,omitempty"`
	Path       string            `json:"path"`                // URL path
	Secret     string            `json:"secret,omitempty"`    // Secret for validation
	Transition string            `json:"transition"`          // Transition to fire
	Map        map[string]string `json:"map,omitempty"`       // Field mapping from payload
	Condition  string            `json:"condition,omitempty"` // Condition for processing
	Method     string            `json:"method,omitempty"`    // HTTP method
}

// Document represents a document/PDF generation configuration.
type Document struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Template    string `json:"template"`           // Template file or inline template
	Format      string `json:"format,omitempty"`   // Output format: pdf, html, docx
	Trigger     string `json:"trigger,omitempty"`  // Transition that triggers generation
	StoreTo     string `json:"storeTo,omitempty"`  // Blob field to store generated document
	Filename    string `json:"filename,omitempty"` // Filename expression
	Description string `json:"description,omitempty"`
}
