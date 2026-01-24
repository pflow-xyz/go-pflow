package metamodel

// Role defines a named role for access control.
type Role struct {
	ID           string   `json:"id"`
	Name         string   `json:"name,omitempty"`
	Description  string   `json:"description,omitempty"`
	Inherits     []string `json:"inherits,omitempty"`     // Parent role IDs for inheritance
	DynamicGrant string   `json:"dynamicGrant,omitempty"` // Expression to dynamically grant role
}

// AccessRule defines who can execute a transition.
type AccessRule struct {
	Transition string   `json:"transition"`      // Transition ID or "*" for all
	Roles      []string `json:"roles,omitempty"` // Allowed roles (empty = any authenticated user)
	Guard      string   `json:"guard,omitempty"` // Guard expression (e.g., "user.id == customer_id")
}
