// Package catacombs implements a roguelike dungeon crawler using Petri nets.
package catacombs

import (
	"fmt"
	"math/rand"
)

// NPCType categorizes NPCs
type NPCType int

const (
	NPCMerchant NPCType = iota
	NPCHealer
	NPCQuestGiver
	NPCWanderer
	NPCGuard
	NPCSage
)

// Mood affects dialogue options and prices
type Mood int

const (
	MoodNeutral Mood = iota
	MoodHappy
	MoodSad
	MoodAngry
	MoodFearful
	MoodGreedy
)

// NPC represents a non-player character
type NPC struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        NPCType  `json:"type"`
	X           int      `json:"x"`
	Y           int      `json:"y"`
	Mood        Mood     `json:"mood"`
	Health      int      `json:"health"`
	MaxHealth   int      `json:"max_health"`
	Gold        int      `json:"gold"`
	Inventory   []Item   `json:"inventory"`
	QuestID     string   `json:"quest_id,omitempty"`
	DialogueIdx int      `json:"dialogue_idx"`
	Met         bool     `json:"met"` // Has player talked to this NPC before?
	Hostile     bool     `json:"hostile"`
	Portrait    string   `json:"portrait"` // ASCII art representation
}

// DialogueNode represents a conversation option
type DialogueNode struct {
	ID       string           `json:"id"`
	Speaker  string           `json:"speaker"`
	Text     string           `json:"text"`
	Choices  []DialogueChoice `json:"choices,omitempty"`
	Action   DialogueAction   `json:"action,omitempty"`
	NextID   string           `json:"next_id,omitempty"` // Auto-continue to this node
}

// DialogueChoice represents a player response
type DialogueChoice struct {
	Text       string         `json:"text"`
	NextID     string         `json:"next_id"`
	Condition  *ChoiceCondition `json:"condition,omitempty"`
	Effect     *ChoiceEffect  `json:"effect,omitempty"`
}

// ChoiceCondition determines if a choice is available
type ChoiceCondition struct {
	RequireGold  int    `json:"require_gold,omitempty"`
	RequireItem  string `json:"require_item,omitempty"`
	RequireQuest string `json:"require_quest,omitempty"`
	RequireMood  Mood   `json:"require_mood,omitempty"`
}

// ChoiceEffect is the result of selecting a choice
type ChoiceEffect struct {
	AddGold     int    `json:"add_gold,omitempty"`
	RemoveGold  int    `json:"remove_gold,omitempty"`
	AddItem     string `json:"add_item,omitempty"`
	RemoveItem  string `json:"remove_item,omitempty"`
	StartQuest  string `json:"start_quest,omitempty"`
	SetMood     *Mood  `json:"set_mood,omitempty"`
	SetHostile  bool   `json:"set_hostile,omitempty"`
	Heal        int    `json:"heal,omitempty"`
}

// DialogueAction is a special action during dialogue
type DialogueAction string

const (
	ActionNone     DialogueAction = ""
	ActionShop     DialogueAction = "shop"
	ActionHeal     DialogueAction = "heal"
	ActionQuest    DialogueAction = "quest"
	ActionEnd      DialogueAction = "end"
	ActionFight    DialogueAction = "fight"
)

// Item represents something that can be held
type Item struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        ItemType `json:"type"`
	Value       int      `json:"value"`
	Effect      int      `json:"effect"` // Healing amount, damage, etc.
	Description string   `json:"description"`
}

// ItemType categorizes items
type ItemType int

const (
	ItemWeapon ItemType = iota
	ItemArmor
	ItemPotion
	ItemKey
	ItemScroll
	ItemGold
	ItemQuest
)

// Quest represents a task for the player
type Quest struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	GiverID     string      `json:"giver_id"`
	Status      QuestStatus `json:"status"`
	Objective   string      `json:"objective"`
	Target      string      `json:"target"` // Enemy type, item, or location
	Required    int         `json:"required"`
	Progress    int         `json:"progress"`
	RewardGold  int         `json:"reward_gold"`
	RewardItem  string      `json:"reward_item,omitempty"`
	RewardXP    int         `json:"reward_xp"`
}

// QuestStatus tracks quest completion
type QuestStatus int

const (
	QuestNotStarted QuestStatus = iota
	QuestActive
	QuestComplete
	QuestFailed
	QuestTurnedIn
)

// NPCTemplates provides base NPC configurations
var NPCTemplates = map[NPCType]struct {
	Names     []string
	Portraits []string
	BaseGold  int
}{
	NPCMerchant: {
		Names:     []string{"Grimbold", "Nessa", "Torvin", "Mira", "Baldric"},
		Portraits: []string{"[M]", "[S]", "[T]"},
		BaseGold:  100,
	},
	NPCHealer: {
		Names:     []string{"Sister Elara", "Brother Kael", "Sage Morrigan", "Priestess Yara"},
		Portraits: []string{"[+]", "[H]"},
		BaseGold:  50,
	},
	NPCQuestGiver: {
		Names:     []string{"Elder Thorne", "Captain Vex", "Mysterious Stranger", "Lady Ashworth"},
		Portraits: []string{"[!]", "[?]"},
		BaseGold:  25,
	},
	NPCWanderer: {
		Names:     []string{"A Lost Soul", "Traveler", "Refugee", "Escaped Prisoner"},
		Portraits: []string{"[o]", "[*]"},
		BaseGold:  10,
	},
	NPCGuard: {
		Names:     []string{"Guard", "Sentinel", "Watcher", "Keeper"},
		Portraits: []string{"[G]", "[!]"},
		BaseGold:  30,
	},
	NPCSage: {
		Names:     []string{"Archmage Zephyr", "Lorekeeper Dust", "Oracle", "The Hermit"},
		Portraits: []string{"[~]", "[@]"},
		BaseGold:  75,
	},
}

// GenerateNPC creates a random NPC of the given type
func GenerateNPC(rng *rand.Rand, npcType NPCType, id string, x, y int) *NPC {
	template := NPCTemplates[npcType]

	name := template.Names[rng.Intn(len(template.Names))]
	portrait := template.Portraits[rng.Intn(len(template.Portraits))]

	// Random mood, weighted toward neutral
	mood := MoodNeutral
	if rng.Float64() < 0.3 {
		mood = Mood(rng.Intn(6))
	}

	// Generate inventory for merchants
	var inventory []Item
	if npcType == NPCMerchant {
		inventory = generateMerchantInventory(rng)
	} else if npcType == NPCHealer {
		inventory = generateHealerInventory(rng)
	}

	return &NPC{
		ID:        id,
		Name:      name,
		Type:      npcType,
		X:         x,
		Y:         y,
		Mood:      mood,
		Health:    100,
		MaxHealth: 100,
		Gold:      template.BaseGold + rng.Intn(50),
		Inventory: inventory,
		Portrait:  portrait,
	}
}

func generateMerchantInventory(rng *rand.Rand) []Item {
	items := []Item{
		{ID: "health_potion", Name: "Health Potion", Type: ItemPotion, Value: 25, Effect: 30, Description: "Restores 30 health"},
		{ID: "torch", Name: "Torch", Type: ItemQuest, Value: 5, Effect: 0, Description: "Lights the way"},
		{ID: "rope", Name: "Rope", Type: ItemQuest, Value: 10, Effect: 0, Description: "50 feet of sturdy rope"},
		{ID: "dagger", Name: "Rusty Dagger", Type: ItemWeapon, Value: 15, Effect: 5, Description: "A simple weapon"},
		{ID: "leather_armor", Name: "Leather Armor", Type: ItemArmor, Value: 40, Effect: 2, Description: "+2 defense"},
	}

	// Random subset
	result := make([]Item, 0)
	for _, item := range items {
		if rng.Float64() < 0.7 {
			result = append(result, item)
		}
	}
	return result
}

func generateHealerInventory(rng *rand.Rand) []Item {
	items := []Item{
		{ID: "health_potion", Name: "Health Potion", Type: ItemPotion, Value: 20, Effect: 30, Description: "Restores 30 health"},
		{ID: "greater_potion", Name: "Greater Health Potion", Type: ItemPotion, Value: 50, Effect: 75, Description: "Restores 75 health"},
		{ID: "antidote", Name: "Antidote", Type: ItemPotion, Value: 15, Effect: 0, Description: "Cures poison"},
	}

	result := make([]Item, 0)
	for _, item := range items {
		if rng.Float64() < 0.8 {
			result = append(result, item)
		}
	}
	return result
}

// GetDialogue returns the dialogue tree for an NPC
func GetDialogue(npc *NPC) []DialogueNode {
	switch npc.Type {
	case NPCMerchant:
		return getMerchantDialogue(npc)
	case NPCHealer:
		return getHealerDialogue(npc)
	case NPCQuestGiver:
		return getQuestGiverDialogue(npc)
	case NPCWanderer:
		return getWandererDialogue(npc)
	case NPCSage:
		return getSageDialogue(npc)
	default:
		return getGenericDialogue(npc)
	}
}

func getMerchantDialogue(npc *NPC) []DialogueNode {
	greeting := "Welcome, traveler!"
	if npc.Mood == MoodGreedy {
		greeting = "Ah, a customer! I have many... valuable wares."
	} else if npc.Mood == MoodFearful {
		greeting = "Please... take what you need, just don't hurt me!"
	} else if !npc.Met {
		greeting = fmt.Sprintf("Greetings! I am %s, humble merchant of these depths.", npc.Name)
	}

	return []DialogueNode{
		{
			ID:      "start",
			Speaker: npc.Name,
			Text:    greeting,
			Choices: []DialogueChoice{
				{Text: "Show me your wares.", NextID: "shop"},
				{Text: "What can you tell me about this place?", NextID: "info"},
				{Text: "Farewell.", NextID: "end"},
			},
		},
		{
			ID:      "shop",
			Speaker: npc.Name,
			Text:    "Of course! Take a look...",
			Action:  ActionShop,
			NextID:  "after_shop",
		},
		{
			ID:      "after_shop",
			Speaker: npc.Name,
			Text:    "Anything else?",
			Choices: []DialogueChoice{
				{Text: "Tell me about this place.", NextID: "info"},
				{Text: "That's all for now.", NextID: "end"},
			},
		},
		{
			ID:      "info",
			Speaker: npc.Name,
			Text:    "These catacombs are ancient... older than any kingdom above. They say treasures lie in the deep, but so do horrors.",
			NextID:  "info2",
		},
		{
			ID:      "info2",
			Speaker: npc.Name,
			Text:    "Watch for locked doors - you'll need keys from the guardians. And beware the lower levels...",
			Choices: []DialogueChoice{
				{Text: "I'll take my chances. Show me your wares.", NextID: "shop"},
				{Text: "Thanks for the warning.", NextID: "end"},
			},
		},
		{
			ID:      "end",
			Speaker: npc.Name,
			Text:    "Safe travels, adventurer.",
			Action:  ActionEnd,
		},
	}
}

func getHealerDialogue(npc *NPC) []DialogueNode {
	greeting := "Peace be upon you, weary soul."
	if npc.Mood == MoodSad {
		greeting = "*sighs* So many wounded... so few I can save. How may I help you?"
	}

	return []DialogueNode{
		{
			ID:      "start",
			Speaker: npc.Name,
			Text:    greeting,
			Choices: []DialogueChoice{
				{Text: "I need healing.", NextID: "heal"},
				{Text: "Do you have any potions?", NextID: "shop"},
				{Text: "What happened here?", NextID: "lore"},
				{Text: "I must go.", NextID: "end"},
			},
		},
		{
			ID:      "heal",
			Speaker: npc.Name,
			Text:    "Let me tend to your wounds... (Healing costs 10 gold)",
			Action:  ActionHeal,
			Choices: []DialogueChoice{
				{
					Text:   "Please heal me. [10 gold]",
					NextID: "healed",
					Condition: &ChoiceCondition{RequireGold: 10},
					Effect:    &ChoiceEffect{RemoveGold: 10, Heal: 50},
				},
				{Text: "Perhaps later.", NextID: "start"},
			},
		},
		{
			ID:      "healed",
			Speaker: npc.Name,
			Text:    "There... the light has mended your flesh. Go carefully.",
			NextID:  "start",
		},
		{
			ID:      "shop",
			Speaker: npc.Name,
			Text:    "I have some remedies...",
			Action:  ActionShop,
			NextID:  "start",
		},
		{
			ID:      "lore",
			Speaker: npc.Name,
			Text:    "A great evil awakened in the depths. Many fled, many died. I stayed to help those I could.",
			Choices: []DialogueChoice{
				{Text: "What evil?", NextID: "lore2"},
				{Text: "You're brave.", NextID: "end"},
			},
		},
		{
			ID:      "lore2",
			Speaker: npc.Name,
			Text:    "They call it the Hollow King. A lich of terrible power. It commands the undead that now roam these halls.",
			NextID:  "start",
		},
		{
			ID:      "end",
			Speaker: npc.Name,
			Text:    "May the light guide your path.",
			Action:  ActionEnd,
		},
	}
}

func getQuestGiverDialogue(npc *NPC) []DialogueNode {
	return []DialogueNode{
		{
			ID:      "start",
			Speaker: npc.Name,
			Text:    "You there! You look capable. I have a task that needs doing.",
			Choices: []DialogueChoice{
				{Text: "What kind of task?", NextID: "quest_offer"},
				{Text: "Not interested.", NextID: "decline"},
			},
		},
		{
			ID:      "quest_offer",
			Speaker: npc.Name,
			Text:    "Skeletons have overrun the eastern chambers. Clear them out - slay 5 of the creatures - and I'll reward you handsomely.",
			Action:  ActionQuest,
			Choices: []DialogueChoice{
				{
					Text:   "I accept.",
					NextID: "accept",
					Effect: &ChoiceEffect{StartQuest: "clear_skeletons"},
				},
				{Text: "What's in it for me?", NextID: "reward"},
				{Text: "Find someone else.", NextID: "decline"},
			},
		},
		{
			ID:      "reward",
			Speaker: npc.Name,
			Text:    "50 gold and a magic amulet I've been holding onto. Fair enough?",
			Choices: []DialogueChoice{
				{
					Text:   "Deal.",
					NextID: "accept",
					Effect: &ChoiceEffect{StartQuest: "clear_skeletons"},
				},
				{Text: "I'll think about it.", NextID: "end"},
			},
		},
		{
			ID:      "accept",
			Speaker: npc.Name,
			Text:    "Excellent! Return to me when the job is done.",
			Action:  ActionEnd,
		},
		{
			ID:      "decline",
			Speaker: npc.Name,
			Text:    "Hmph. Coward. Begone then.",
			Action:  ActionEnd,
		},
		{
			ID:      "end",
			Speaker: npc.Name,
			Text:    "We'll speak again.",
			Action:  ActionEnd,
		},
	}
}

func getWandererDialogue(npc *NPC) []DialogueNode {
	return []DialogueNode{
		{
			ID:      "start",
			Speaker: npc.Name,
			Text:    "*looks up nervously* Oh! Another living soul... I thought I was alone down here.",
			Choices: []DialogueChoice{
				{Text: "Are you alright?", NextID: "story"},
				{Text: "How did you end up here?", NextID: "story"},
				{Text: "I can't help you.", NextID: "abandon"},
			},
		},
		{
			ID:      "story",
			Speaker: npc.Name,
			Text:    "I was with an expedition... we sought the legendary treasure. But we were ambushed. I ran... I've been hiding ever since.",
			Choices: []DialogueChoice{
				{Text: "Do you know a way out?", NextID: "directions"},
				{Text: "What treasure?", NextID: "treasure"},
				{Text: "Stay safe.", NextID: "end"},
			},
		},
		{
			ID:      "directions",
			Speaker: npc.Name,
			Text:    "The stairs up should be to the north... but there are guards. You'll need to find another path or fight through.",
			NextID:  "end",
		},
		{
			ID:      "treasure",
			Speaker: npc.Name,
			Text:    "The Crown of the Hollow King. They say whoever wears it gains power over death itself. But the price...",
			NextID:  "end",
		},
		{
			ID:      "abandon",
			Speaker: npc.Name,
			Text:    "*whimpers* Please... at least tell me which way is safe...",
			Action:  ActionEnd,
		},
		{
			ID:      "end",
			Speaker: npc.Name,
			Text:    "Good luck out there...",
			Action:  ActionEnd,
		},
	}
}

func getSageDialogue(npc *NPC) []DialogueNode {
	return []DialogueNode{
		{
			ID:      "start",
			Speaker: npc.Name,
			Text:    "*speaks without looking up* The threads of fate have brought you here. What do you seek?",
			Choices: []DialogueChoice{
				{Text: "Knowledge.", NextID: "knowledge"},
				{Text: "Power.", NextID: "power"},
				{Text: "A way out.", NextID: "escape"},
				{Text: "Nothing from you.", NextID: "dismiss"},
			},
		},
		{
			ID:      "knowledge",
			Speaker: npc.Name,
			Text:    "Wise. The catacombs predate the kingdoms above by millennia. Built by a civilization that transcended death... and were destroyed by it.",
			NextID:  "knowledge2",
		},
		{
			ID:      "knowledge2",
			Speaker: npc.Name,
			Text:    "Their king refused to die. He bound his soul to these stones. Now he is the Hollow King, and these are his eternal halls.",
			Choices: []DialogueChoice{
				{Text: "How do I defeat him?", NextID: "defeat"},
				{Text: "Thank you for the lesson.", NextID: "end"},
			},
		},
		{
			ID:      "power",
			Speaker: npc.Name,
			Text:    "Power comes at a price. The crown below offers much... but takes everything. Many have sought it. None have returned unchanged.",
			NextID:  "end",
		},
		{
			ID:      "escape",
			Speaker: npc.Name,
			Text:    "There is no escape from what you carry within. But if you mean these halls... the stairs are guarded. Or you could go deeper, through the throne room.",
			NextID:  "end",
		},
		{
			ID:      "defeat",
			Speaker: npc.Name,
			Text:    "The phylactery. His soul is bound to an object deep in the throne room. Destroy it, and the Hollow King falls. But be warned - it is well protected.",
			NextID:  "end",
		},
		{
			ID:      "dismiss",
			Speaker: npc.Name,
			Text:    "*chuckles* You will need me eventually. They all do.",
			Action:  ActionEnd,
		},
		{
			ID:      "end",
			Speaker: npc.Name,
			Text:    "The threads wind ever onward...",
			Action:  ActionEnd,
		},
	}
}

func getGenericDialogue(npc *NPC) []DialogueNode {
	return []DialogueNode{
		{
			ID:      "start",
			Speaker: npc.Name,
			Text:    "...",
			Choices: []DialogueChoice{
				{Text: "Hello?", NextID: "response"},
				{Text: "Never mind.", NextID: "end"},
			},
		},
		{
			ID:      "response",
			Speaker: npc.Name,
			Text:    "*stares silently*",
			Action:  ActionEnd,
		},
		{
			ID:      "end",
			Speaker: npc.Name,
			Text:    "...",
			Action:  ActionEnd,
		},
	}
}

// NPCToRune returns the ASCII character for an NPC
func NPCToRune(npc *NPC) rune {
	if npc.Hostile {
		return '!'
	}
	switch npc.Type {
	case NPCMerchant:
		return '$'
	case NPCHealer:
		return '+'
	case NPCQuestGiver:
		return '?'
	case NPCGuard:
		return 'G'
	case NPCSage:
		return '@'
	default:
		return 'N'
	}
}

// NPCTypeName returns the display name for an NPC type
func NPCTypeName(t NPCType) string {
	names := []string{"Merchant", "Healer", "Quest Giver", "Wanderer", "Guard", "Sage"}
	if int(t) < len(names) {
		return names[t]
	}
	return "Unknown"
}

// MoodName returns the display name for a mood
func MoodName(m Mood) string {
	names := []string{"Neutral", "Happy", "Sad", "Angry", "Fearful", "Greedy"}
	if int(m) < len(names) {
		return names[m]
	}
	return "Unknown"
}
