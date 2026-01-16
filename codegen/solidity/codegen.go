// Package solidity generates Solidity smart contracts from metamodel schemas.
package solidity

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// Generate produces a Solidity contract from a metamodel schema.
func Generate(schema *metamodel.Schema) string {
	g := &generator{schema: schema}
	return g.generate()
}

type generator struct {
	schema *metamodel.Schema
}

func (g *generator) generate() string {
	var b strings.Builder

	// SPDX and pragma
	b.WriteString("// SPDX-License-Identifier: MIT\n")
	b.WriteString("pragma solidity ^0.8.20;\n\n")

	// Contract comment
	b.WriteString(fmt.Sprintf("/// @title %s\n", g.schema.Name))
	b.WriteString(fmt.Sprintf("/// @notice Generated from arcnet schema (version %s)\n", g.schema.Version))
	b.WriteString("/// @dev This contract was auto-generated from a Petri net model\n")
	b.WriteString("/// @dev IMPORTANT: This contract includes basic access control. Review and enhance for production use.\n\n")

	// Contract declaration
	contractName := toContractName(g.schema.Name)
	b.WriteString(fmt.Sprintf("contract %s {\n", contractName))

	// Generate struct definitions for complex types
	structs := g.generateStructs()
	if structs != "" {
		b.WriteString(structs)
	}

	// Generate access control
	b.WriteString(g.generateAccessControl())

	// Generate state variables
	b.WriteString(g.generateStateVariables())

	// Generate events
	b.WriteString(g.generateEvents())

	// Generate constructor
	b.WriteString(g.generateConstructor())

	// Generate functions for each action
	for _, action := range g.schema.Actions {
		b.WriteString(g.generateFunction(action))
	}

	// Generate epoch functions for time-based features
	b.WriteString(g.generateEpochFunctions())

	// Generate view functions for exported states
	b.WriteString(g.generateViewFunctions())

	// Generate admin functions
	b.WriteString(g.generateAdminFunctions())

	b.WriteString("}\n")

	return b.String()
}

func (g *generator) generateStructs() string {
	var b strings.Builder
	seen := make(map[string]bool)

	for _, state := range g.schema.States {
		// Check for struct types like map[uint256]VestingSchedule
		if strings.Contains(state.Type, "VestingSchedule") && !seen["VestingSchedule"] {
			seen["VestingSchedule"] = true
			b.WriteString("    struct VestingSchedule {\n")
			b.WriteString("        uint256 start;\n")
			b.WriteString("        uint256 cliff;\n")
			b.WriteString("        uint256 end;\n")
			b.WriteString("        uint256 total;\n")
			b.WriteString("        bool revocable;\n")
			b.WriteString("    }\n\n")
		}
	}

	return b.String()
}

func (g *generator) generateAccessControl() string {
	var b strings.Builder

	b.WriteString("    // ============ Access Control ============\n")
	b.WriteString("    // Basic ownership model. For production, consider OpenZeppelin's Ownable or AccessControl.\n\n")
	b.WriteString("    address public contractOwner;\n\n")
	b.WriteString("    error Unauthorized();\n")
	b.WriteString("    error ZeroAddress();\n\n")
	b.WriteString("    modifier onlyOwner() {\n")
	b.WriteString("        if (msg.sender != contractOwner) revert Unauthorized();\n")
	b.WriteString("        _;\n")
	b.WriteString("    }\n\n")
	b.WriteString("    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);\n\n")

	return b.String()
}

func (g *generator) generateConstructor() string {
	var b strings.Builder

	b.WriteString("    // ============ Constructor ============\n\n")
	b.WriteString("    constructor() {\n")
	b.WriteString("        contractOwner = msg.sender;\n")
	b.WriteString("        emit OwnershipTransferred(address(0), msg.sender);\n")
	b.WriteString("    }\n\n")

	return b.String()
}

func (g *generator) generateAdminFunctions() string {
	var b strings.Builder

	b.WriteString("    // ============ Admin Functions ============\n\n")
	b.WriteString("    /// @notice Transfer ownership to a new address\n")
	b.WriteString("    /// @param newOwner The address to transfer ownership to\n")
	b.WriteString("    function transferOwnership(address newOwner) external onlyOwner {\n")
	b.WriteString("        if (newOwner == address(0)) revert ZeroAddress();\n")
	b.WriteString("        emit OwnershipTransferred(contractOwner, newOwner);\n")
	b.WriteString("        contractOwner = newOwner;\n")
	b.WriteString("    }\n\n")
	b.WriteString("    /// @notice Renounce ownership (BE CAREFUL - this is irreversible)\n")
	b.WriteString("    function renounceOwnership() external onlyOwner {\n")
	b.WriteString("        emit OwnershipTransferred(contractOwner, address(0));\n")
	b.WriteString("        contractOwner = address(0);\n")
	b.WriteString("    }\n\n")

	// Testing helpers
	b.WriteString("    // ============ Testing Helpers ============\n")
	b.WriteString("    // WARNING: These functions are for local development/testing only.\n")
	b.WriteString("    // NEVER deploy a contract with these functions to mainnet or any network with real value.\n\n")
	b.WriteString("    address internal originalOwner;\n")
	b.WriteString("    address internal previousOwner;\n\n")
	b.WriteString("    /// @notice Allows any caller to become the contract owner (TESTING ONLY)\n")
	b.WriteString("    /// @dev This is a backdoor for local Anvil testing - DO NOT USE IN PRODUCTION\n")
	b.WriteString("    function unsafeBecomeOwner() external {\n")
	b.WriteString("        if (originalOwner == address(0)) {\n")
	b.WriteString("            originalOwner = contractOwner;\n")
	b.WriteString("        }\n")
	b.WriteString("        previousOwner = contractOwner;\n")
	b.WriteString("        emit OwnershipTransferred(contractOwner, msg.sender);\n")
	b.WriteString("        contractOwner = msg.sender;\n")
	b.WriteString("    }\n\n")
	b.WriteString("    /// @notice Restores the previous owner (TESTING ONLY)\n")
	b.WriteString("    /// @dev This is a backdoor for local Anvil testing - DO NOT USE IN PRODUCTION\n")
	b.WriteString("    function unsafeRestoreOwner() external {\n")
	b.WriteString("        emit OwnershipTransferred(contractOwner, previousOwner);\n")
	b.WriteString("        contractOwner = previousOwner;\n")
	b.WriteString("    }\n\n")
	b.WriteString("    /// @notice Restores the original owner from deployment (TESTING ONLY)\n")
	b.WriteString("    /// @dev This is a backdoor for local Anvil testing - DO NOT USE IN PRODUCTION\n")
	b.WriteString("    function unsafeRestoreOriginalOwner() external {\n")
	b.WriteString("        emit OwnershipTransferred(contractOwner, originalOwner);\n")
	b.WriteString("        contractOwner = originalOwner;\n")
	b.WriteString("    }\n\n")

	return b.String()
}

// isPrivilegedAction returns true if an action should require owner authorization
// Only truly admin functions should be here - not functions that have their own permission models
func isPrivilegedAction(actionID string) bool {
	privileged := map[string]bool{
		// Minting creates new supply - should be admin only
		"mint":      true,
		"tokenMint": true,
		// Vault yield comes from external source - admin adds it
		"vaultHarvest": true,
		// Note: tokenBurn, burn, vestCreate, vestRevoke have their own authorization models
		// (token holder, approved operator, vesting creator, etc.)
	}
	return privileged[actionID]
}

func (g *generator) generateStateVariables() string {
	var b strings.Builder
	b.WriteString("    // ============ State Variables ============\n\n")

	// Add epoch support if schema uses time-based features
	if g.hasTimeBasedFeatures() {
		b.WriteString("    uint256 public currentEpoch;\n")
	}

	// Add event sequence counter for debugging
	b.WriteString("    uint256 internal eventSequence;\n")

	for _, state := range g.schema.States {
		solType := toSolidityType(state.Type)
		visibility := "internal"
		if state.Exported {
			visibility = "public"
		}
		b.WriteString(fmt.Sprintf("    %s %s %s;\n", solType, visibility, state.ID))
	}

	b.WriteString("\n")
	return b.String()
}

// hasTimeBasedFeatures checks if the schema has vesting or other time-dependent features
func (g *generator) hasTimeBasedFeatures() bool {
	for _, state := range g.schema.States {
		if strings.Contains(state.ID, "vest") || strings.Contains(state.Type, "VestingSchedule") {
			return true
		}
	}
	return false
}

// generateEpochGuards generates epoch-based require statements for time-sensitive functions
func (g *generator) generateEpochGuards(actionID string) string {
	if !g.hasTimeBasedFeatures() {
		return ""
	}

	var b strings.Builder

	switch actionID {
	case "vestClaim":
		// Require that cliff has been reached and claimAmount <= claimable
		b.WriteString("        require(claimAmount <= claimableAmount(tokenId), \"exceeds claimable\");\n\n")

	case "vestRevoke":
		// Require that the schedule is revocable
		b.WriteString("        require(vestSchedules[tokenId].revocable, \"not revocable\");\n\n")
	}

	return b.String()
}

func (g *generator) generateEvents() string {
	var b strings.Builder
	b.WriteString("    // ============ Events ============\n\n")

	for _, action := range g.schema.Actions {
		params := g.inferEventParams(action)
		// Add epoch and seq to all events for debugging
		if params != "" {
			params = "uint256 epoch, uint256 seq, " + params
		} else {
			params = "uint256 epoch, uint256 seq"
		}
		b.WriteString(fmt.Sprintf("    event %s(%s);\n", toEventName(action.ID), params))
	}

	b.WriteString("\n")
	return b.String()
}

func (g *generator) inferEventParams(action metamodel.Action) string {
	// Collect unique parameters from arcs
	params := make(map[string]string)

	for _, arc := range g.schema.InputArcs(action.ID) {
		for _, key := range arc.Keys {
			params[key] = inferParamType(key)
		}
		if arc.Value != "" {
			params[arc.Value] = g.inferValueType(arc)
		}
	}

	for _, arc := range g.schema.OutputArcs(action.ID) {
		for _, key := range arc.Keys {
			params[key] = inferParamType(key)
		}
		if arc.Value != "" {
			params[arc.Value] = g.inferValueType(arc)
		}
	}

	// Build param list
	var parts []string
	// Order: common params first
	order := []string{"caller", "from", "to", "owner", "spender", "operator", "receiver", "beneficiary", "id", "tokenId", "amount", "assets", "shares"}
	seen := make(map[string]bool)

	for _, name := range order {
		if typ, ok := params[name]; ok {
			indexed := ""
			if name == "from" || name == "to" || name == "owner" || name == "caller" {
				indexed = " indexed"
			}
			parts = append(parts, fmt.Sprintf("%s%s %s", typ, indexed, name))
			seen[name] = true
		}
	}

	// Add remaining
	for name, typ := range params {
		if !seen[name] {
			parts = append(parts, fmt.Sprintf("%s %s", typ, name))
		}
	}

	return strings.Join(parts, ", ")
}

func (g *generator) generateFunction(action metamodel.Action) string {
	var b strings.Builder

	funcName := action.ID

	// Generate storage operations from arcs first (to detect needed params)
	inputOps, outputOps := g.generateArcOperations(action.ID)

	// Get base params from arcs and guards (using AST-based extraction)
	params := g.inferFunctionParamsWithBody(action, inputOps, outputOps)

	// Determine if function needs access control
	modifier := ""
	if isPrivilegedAction(action.ID) {
		modifier = " onlyOwner"
	}

	b.WriteString(fmt.Sprintf("    // ============ %s ============\n\n", funcName))
	b.WriteString(fmt.Sprintf("    function %s(%s) external%s {\n", funcName, params, modifier))

	// Generate require statements from guard using AST-based translator
	if action.Guard != "" {
		translator := NewGuardTranslator()
		requires, err := translator.TranslateGuard(action.Guard)
		if err != nil {
			// Fallback to legacy string-based translation
			requires = translateGuard(action.Guard)
		}
		for _, req := range requires {
			b.WriteString(fmt.Sprintf("        %s\n", req))
		}
		if len(requires) > 0 {
			b.WriteString("\n")
		}
	}

	// Add epoch-based guards for vesting functions
	epochGuards := g.generateEpochGuards(action.ID)
	if epochGuards != "" {
		b.WriteString(epochGuards)
	}

	// Input arcs (decrements/reads)
	for _, op := range inputOps {
		b.WriteString(fmt.Sprintf("        %s\n", op))
	}

	// Output arcs (increments/writes)
	for _, op := range outputOps {
		b.WriteString(fmt.Sprintf("        %s\n", op))
	}

	// Emit event with epoch and sequence for debugging
	eventParams := g.inferEventArgs(action)
	if eventParams != "" {
		eventParams = "currentEpoch, eventSequence++, " + eventParams
	} else {
		eventParams = "currentEpoch, eventSequence++"
	}
	b.WriteString(fmt.Sprintf("\n        emit %s(%s);\n", toEventName(action.ID), eventParams))

	b.WriteString("    }\n\n")

	return b.String()
}

func (g *generator) inferFunctionParamsWithBody(action metamodel.Action, inputs, outputs []string) string {
	params := g.collectFunctionParams(action)

	// Add params used in body but not declared
	bodyParams := extractBodyParams(inputs, outputs)
	for name, typ := range bodyParams {
		if _, exists := params[name]; !exists {
			params[name] = typ
		}
	}

	return g.formatParams(params)
}

func (g *generator) inferFunctionParams(action metamodel.Action) string {
	params := g.collectFunctionParams(action)
	return g.formatParams(params)
}

func (g *generator) collectFunctionParams(action metamodel.Action) map[string]string {
	params := make(map[string]string)

	for _, arc := range g.schema.InputArcs(action.ID) {
		for _, key := range arc.Keys {
			params[key] = inferParamType(key)
		}
		if arc.Value != "" {
			params[arc.Value] = inferParamType(arc.Value)
		}
	}

	for _, arc := range g.schema.OutputArcs(action.ID) {
		for _, key := range arc.Keys {
			params[key] = inferParamType(key)
		}
		if arc.Value != "" {
			params[arc.Value] = inferParamType(arc.Value)
		}
	}

	// Extract parameters from guard expressions
	if action.Guard != "" {
		guardParams := extractGuardParams(action.Guard)
		for name, typ := range guardParams {
			if _, exists := params[name]; !exists {
				params[name] = typ
			}
		}
	}

	// Remove 'caller' - we use msg.sender
	delete(params, "caller")

	return params
}

func (g *generator) formatParams(params map[string]string) string {
	var parts []string
	order := []string{"from", "to", "owner", "spender", "operator", "receiver", "beneficiary", "id", "tokenId", "amount", "assets", "shares", "approved", "nftAmount", "claimAmount", "unvestedAmount", "yieldAmount", "total", "start", "cliff", "end", "revocable"}
	seen := make(map[string]bool)

	for _, name := range order {
		if typ, ok := params[name]; ok {
			parts = append(parts, fmt.Sprintf("%s %s", typ, name))
			seen[name] = true
		}
	}

	for name, typ := range params {
		if !seen[name] {
			parts = append(parts, fmt.Sprintf("%s %s", typ, name))
		}
	}

	return strings.Join(parts, ", ")
}

// extractGuardParams finds parameter names used in guard expressions using AST parsing.
func extractGuardParams(guard string) map[string]string {
	if guard == "" {
		return nil
	}

	// Use AST-based parameter extraction
	translator := NewGuardTranslator()
	params, err := translator.ExtractParameters(guard)
	if err != nil {
		// Fallback to simple pattern matching
		params = make(map[string]string)
		knownParams := []string{"amount", "from", "to", "owner", "spender", "id", "tokenId"}
		for _, param := range knownParams {
			if strings.Contains(guard, param) {
				params[param] = inferParamType(param)
			}
		}
	}

	return params
}

// extractBodyParams finds parameter names used in function body that need to be declared
func extractBodyParams(inputs, outputs []string) map[string]string {
	params := make(map[string]string)

	// Known parameter names that might appear in generated code
	knownParams := map[string]string{
		"amount":         "uint256",
		"start":          "uint256",
		"cliff":          "uint256",
		"end":            "uint256",
		"total":          "uint256",
		"revocable":      "bool",
		"assets":         "uint256",
		"shares":         "uint256",
		"nftAmount":      "uint256",
		"claimAmount":    "uint256",
		"unvestedAmount": "uint256",
		"yieldAmount":    "uint256",
	}

	allCode := strings.Join(append(inputs, outputs...), " ")

	for param, typ := range knownParams {
		// Check if param is used as a whole word (not as part of a larger word like "sender")
		if containsWord(allCode, param) {
			params[param] = typ
		}
	}

	return params
}

// containsWord checks if code contains param as a whole word
func containsWord(code, word string) bool {
	idx := 0
	for {
		pos := strings.Index(code[idx:], word)
		if pos == -1 {
			return false
		}
		pos += idx

		// Check character before
		if pos > 0 {
			before := code[pos-1]
			if isWordChar(before) {
				idx = pos + 1
				continue
			}
		}

		// Check character after
		endPos := pos + len(word)
		if endPos < len(code) {
			after := code[endPos]
			if isWordChar(after) {
				idx = pos + 1
				continue
			}
		}

		return true
	}
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func (g *generator) inferEventArgs(action metamodel.Action) string {
	params := make(map[string]bool)

	for _, arc := range g.schema.InputArcs(action.ID) {
		for _, key := range arc.Keys {
			params[key] = true
		}
		if arc.Value != "" {
			params[arc.Value] = true
		}
	}

	for _, arc := range g.schema.OutputArcs(action.ID) {
		for _, key := range arc.Keys {
			params[key] = true
		}
		if arc.Value != "" {
			params[arc.Value] = true
		}
	}

	var parts []string
	order := []string{"caller", "from", "to", "owner", "spender", "operator", "receiver", "beneficiary", "id", "tokenId", "amount", "assets", "shares", "approved", "nftAmount", "claimAmount", "unvestedAmount", "yieldAmount", "total"}
	seen := make(map[string]bool)

	for _, name := range order {
		if params[name] {
			if name == "caller" {
				parts = append(parts, "msg.sender")
			} else {
				parts = append(parts, name)
			}
			seen[name] = true
		}
	}

	for name := range params {
		if !seen[name] {
			parts = append(parts, name)
		}
	}

	return strings.Join(parts, ", ")
}

func (g *generator) generateArcOperations(actionID string) (inputs []string, outputs []string) {
	inputArcs := g.schema.InputArcs(actionID)
	outputArcs := g.schema.OutputArcs(actionID)

	for _, arc := range inputArcs {
		state := g.schema.StateByID(arc.Source)
		if state == nil {
			continue
		}

		accessor := buildAccessor(arc.Source, arc.Keys)
		value := arc.Value
		if value == "" {
			value = "amount"
		}

		if isMapType(state.Type) {
			// Check value type
			valueType := getMapValueType(state.Type)
			if strings.Contains(valueType, "VestingSchedule") {
				// Struct deletion
				inputs = append(inputs, fmt.Sprintf("delete %s;", accessor))
			} else if strings.Contains(valueType, "uint256") {
				// Decrement for numeric maps
				inputs = append(inputs, fmt.Sprintf("%s -= %s;", accessor, value))
			}
		} else {
			// Simple state decrement
			inputs = append(inputs, fmt.Sprintf("%s -= %s;", arc.Source, value))
		}
	}

	for _, arc := range outputArcs {
		state := g.schema.StateByID(arc.Target)
		if state == nil {
			continue
		}

		accessor := buildAccessor(arc.Target, arc.Keys)
		value := arc.Value
		if value == "" {
			value = "amount"
		}

		if isMapType(state.Type) {
			valueType := getMapValueType(state.Type)
			if valueType == "bool" {
				// Boolean maps: direct assignment
				outputs = append(outputs, fmt.Sprintf("%s = %s;", accessor, translateValue(value)))
			} else if valueType == "address" {
				// Address value maps (like tokenApproved, vestCreators): direct assignment
				outputs = append(outputs, fmt.Sprintf("%s = %s;", accessor, translateValue(value)))
			} else if strings.Contains(valueType, "VestingSchedule") {
				// Struct assignment
				outputs = append(outputs, fmt.Sprintf("%s = VestingSchedule(start, cliff, end, total, revocable);", accessor))
			} else if strings.Contains(valueType, "uint256") {
				// Increment for numeric maps
				outputs = append(outputs, fmt.Sprintf("%s += %s;", accessor, value))
			}
		} else {
			// Simple state increment
			outputs = append(outputs, fmt.Sprintf("%s += %s;", arc.Target, value))
		}
	}

	return inputs, outputs
}

// getMapValueType extracts the value type from a map type like "map[address]uint256"
func getMapValueType(mapType string) string {
	re := regexp.MustCompile(`^map\[[^\]]+\](.+)$`)
	matches := re.FindStringSubmatch(mapType)
	if len(matches) == 2 {
		// Recursively get innermost value type
		inner := matches[1]
		if strings.HasPrefix(inner, "map[") {
			return getMapValueType(inner)
		}
		return inner
	}
	return ""
}

// inferValueType determines the Solidity type for an arc's value based on the target state
func (g *generator) inferValueType(arc metamodel.Arc) string {
	// Look up the state this arc connects to
	stateID := arc.Target
	if stateID == "" {
		stateID = arc.Source
	}

	state := g.schema.StateByID(stateID)
	if state == nil {
		return inferParamType(arc.Value)
	}

	// Get the value type from the state's type
	if isMapType(state.Type) {
		valueType := getMapValueType(state.Type)
		switch valueType {
		case "address":
			return "address"
		case "bool":
			return "bool"
		case "uint256":
			return "uint256"
		default:
			if strings.Contains(valueType, "VestingSchedule") {
				return "VestingSchedule memory"
			}
			return "uint256"
		}
	}

	return inferParamType(arc.Value)
}

func (g *generator) generateEpochFunctions() string {
	if !g.hasTimeBasedFeatures() {
		return ""
	}

	var b strings.Builder
	b.WriteString("    // ============ Epoch Functions ============\n\n")

	// advanceEpoch - increments the epoch counter and resets event sequence
	b.WriteString("    /// @notice Advance the epoch counter (admin only)\n")
	b.WriteString("    function advanceEpoch(uint256 epochs) external onlyOwner {\n")
	b.WriteString("        currentEpoch += epochs;\n")
	b.WriteString("        eventSequence = 0;\n")
	b.WriteString("    }\n\n")

	// vestedAmount - calculates vested tokens for a schedule
	b.WriteString("    function vestedAmount(uint256 tokenId) public view returns (uint256) {\n")
	b.WriteString("        VestingSchedule storage s = vestSchedules[tokenId];\n")
	b.WriteString("        if (currentEpoch < s.cliff) {\n")
	b.WriteString("            return 0;\n")
	b.WriteString("        }\n")
	b.WriteString("        if (currentEpoch >= s.end) {\n")
	b.WriteString("            return s.total;\n")
	b.WriteString("        }\n")
	b.WriteString("        // Linear vesting between cliff and end\n")
	b.WriteString("        uint256 elapsed = currentEpoch - s.start;\n")
	b.WriteString("        uint256 duration = s.end - s.start;\n")
	b.WriteString("        return (s.total * elapsed) / duration;\n")
	b.WriteString("    }\n\n")

	// claimableAmount - vested minus already claimed
	b.WriteString("    function claimableAmount(uint256 tokenId) public view returns (uint256) {\n")
	b.WriteString("        uint256 vested = vestedAmount(tokenId);\n")
	b.WriteString("        uint256 claimed = vestClaimed[tokenId];\n")
	b.WriteString("        if (vested <= claimed) {\n")
	b.WriteString("            return 0;\n")
	b.WriteString("        }\n")
	b.WriteString("        return vested - claimed;\n")
	b.WriteString("    }\n\n")

	return b.String()
}

func (g *generator) generateViewFunctions() string {
	var b strings.Builder
	b.WriteString("    // ============ View Functions ============\n\n")

	for _, state := range g.schema.States {
		// Add public getters for internal states used in invariant testing
		// Use "get" prefix to avoid name collision with internal variables
		if !state.Exported {
			switch state.ID {
			case "totalSupply":
				b.WriteString("    function getTotalSupply() external view returns (uint256) {\n")
				b.WriteString("        return totalSupply;\n")
				b.WriteString("    }\n\n")
			case "vaultTotalAssets":
				b.WriteString("    function getVaultTotalAssets() external view returns (uint256) {\n")
				b.WriteString("        return vaultTotalAssets;\n")
				b.WriteString("    }\n\n")
			case "vaultTotalShares":
				b.WriteString("    function getVaultTotalShares() external view returns (uint256) {\n")
				b.WriteString("        return vaultTotalShares;\n")
				b.WriteString("    }\n\n")
			case "vestTotalLocked":
				b.WriteString("    function getVestTotalLocked() external view returns (uint256) {\n")
				b.WriteString("        return vestTotalLocked;\n")
				b.WriteString("    }\n\n")
			}
			continue
		}

		// Public state variables already have auto-generated getters
		// Add helper functions for common queries
		if state.ID == "balances" {
			b.WriteString("    function balanceOf(address account) external view returns (uint256) {\n")
			b.WriteString("        return balances[account];\n")
			b.WriteString("    }\n\n")
		}

		if state.ID == "allowances" {
			b.WriteString("    function allowance(address owner, address spender) external view returns (uint256) {\n")
			b.WriteString("        return allowances[owner][spender];\n")
			b.WriteString("    }\n\n")
		}

		if state.ID == "tokenBalances" {
			b.WriteString("    function balanceOf(address account, uint256 id) external view returns (uint256) {\n")
			b.WriteString("        return tokenBalances[id][account];\n")
			b.WriteString("    }\n\n")
		}

		if state.ID == "operators" {
			b.WriteString("    function isApprovedForAll(address owner, address operator) external view returns (bool) {\n")
			b.WriteString("        return operators[owner][operator];\n")
			b.WriteString("    }\n\n")
		}
	}

	return b.String()
}

// toSolidityType converts arcnet type notation to Solidity type.
func toSolidityType(arcType string) string {
	if arcType == "" || arcType == "uint256" {
		return "uint256"
	}

	// Token states use "int" but Solidity prefers uint256 for counters
	if arcType == "int" {
		return "uint256"
	}

	// Handle nested maps: map[a]map[b]c -> mapping(a => mapping(b => c))
	if strings.HasPrefix(arcType, "map[") {
		return convertMapType(arcType)
	}

	return arcType
}

func convertMapType(arcType string) string {
	// Parse map[keyType]valueType
	re := regexp.MustCompile(`^map\[([^\]]+)\](.+)$`)
	matches := re.FindStringSubmatch(arcType)
	if len(matches) != 3 {
		return arcType
	}

	keyType := matches[1]
	valueType := matches[2]

	// Convert key type
	solKeyType := keyType
	if keyType == "address" {
		solKeyType = "address"
	} else if keyType == "uint256" {
		solKeyType = "uint256"
	}

	// Recursively convert value type
	solValueType := toSolidityType(valueType)

	return fmt.Sprintf("mapping(%s => %s)", solKeyType, solValueType)
}

func isMapType(t string) bool {
	return strings.HasPrefix(t, "map[")
}

func buildAccessor(stateID string, keys []string) string {
	if len(keys) == 0 {
		return stateID
	}

	accessor := stateID
	for _, key := range keys {
		if key == "caller" {
			accessor += "[msg.sender]"
		} else {
			accessor += fmt.Sprintf("[%s]", key)
		}
	}
	return accessor
}

// translateValue replaces 'caller' with 'msg.sender' in value expressions
func translateValue(value string) string {
	if value == "caller" {
		return "msg.sender"
	}
	return value
}

// translateGuard converts an arcnet guard expression to Solidity require statements.
func translateGuard(guard string) []string {
	if guard == "" {
		return nil
	}

	var requires []string

	// Split on && for separate require statements
	parts := strings.Split(guard, "&&")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Translate common patterns
		solExpr := part

		// Replace caller with msg.sender
		solExpr = strings.ReplaceAll(solExpr, "caller", "msg.sender")

		// address(0) is already valid Solidity
		// Comparisons are already valid

		// Generate error message from expression
		errMsg := generateErrorMessage(part)

		requires = append(requires, fmt.Sprintf("require(%s, \"%s\");", solExpr, errMsg))
	}

	return requires
}

func generateErrorMessage(expr string) string {
	expr = strings.TrimSpace(expr)

	// Common patterns
	if strings.Contains(expr, ">= amount") {
		if strings.Contains(expr, "balances[from]") {
			return "insufficient balance"
		}
		if strings.Contains(expr, "allowances") {
			return "insufficient allowance"
		}
	}

	if strings.Contains(expr, "!= address(0)") {
		return "zero address"
	}

	if strings.Contains(expr, "== caller") || strings.Contains(expr, "caller ==") {
		return "not authorized"
	}

	if strings.Contains(expr, "operators") || strings.Contains(expr, "Approved") {
		return "not authorized"
	}

	// Default: use expression as message
	if len(expr) > 40 {
		return "precondition failed"
	}
	return expr
}

func toContractName(name string) string {
	// Remove special characters, capitalize
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}

func toEventName(actionID string) string {
	// Capitalize first letter
	if len(actionID) == 0 {
		return actionID
	}
	return strings.ToUpper(actionID[:1]) + actionID[1:]
}

func inferParamType(name string) string {
	switch name {
	case "from", "to", "owner", "spender", "operator", "receiver", "beneficiary", "caller":
		return "address"
	case "approved":
		return "bool"
	case "id", "tokenId", "amount", "assets", "shares", "nftAmount", "claimAmount", "unvestedAmount", "yieldAmount", "total", "start", "cliff", "end":
		return "uint256"
	case "revocable":
		return "bool"
	default:
		return "uint256"
	}
}
