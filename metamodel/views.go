package metamodel

// View represents a UI view definition for presenting workflow data.
type View struct {
	ID          string      `json:"id"`
	Name        string      `json:"name,omitempty"`
	Kind        string      `json:"kind,omitempty"` // form, card, table, detail
	Description string      `json:"description,omitempty"`
	Groups      []ViewGroup `json:"groups,omitempty"`
	Actions     []string    `json:"actions,omitempty"` // Transition IDs that can be triggered
}

// ViewGroup represents a logical grouping of fields within a view.
type ViewGroup struct {
	ID     string      `json:"id"`
	Name   string      `json:"name,omitempty"`
	Fields []ViewField `json:"fields"`
}

// ViewField represents a single field within a view group.
type ViewField struct {
	Binding     string `json:"binding"`
	Label       string `json:"label,omitempty"`
	Type        string `json:"type,omitempty"` // text, number, select, date, etc.
	Required    bool   `json:"required,omitempty"`
	ReadOnly    bool   `json:"readonly,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// Navigation represents the navigation menu configuration.
type Navigation struct {
	Brand string           `json:"brand"`
	Items []NavigationItem `json:"items"`
}

// NavigationItem represents a single navigation menu item.
type NavigationItem struct {
	Label string   `json:"label"`
	Path  string   `json:"path"`
	Icon  string   `json:"icon,omitempty"`
	Roles []string `json:"roles,omitempty"` // empty = visible to all
}

// Admin represents admin dashboard configuration.
type Admin struct {
	Enabled  bool     `json:"enabled"`
	Path     string   `json:"path"`
	Roles    []string `json:"roles"`
	Features []string `json:"features"` // list, detail, history, transitions
}
