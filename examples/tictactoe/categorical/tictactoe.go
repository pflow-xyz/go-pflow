// Package categorical demonstrates the struct tag DSL for Tic-tac-toe.
// The game is defined by its objects (places) and morphisms (flows).
package categorical

import "github.com/pflow-xyz/go-pflow/metamodel/dsl"

// TicTacToe defines the game as a categorical schema.
// Objects are states, morphisms are flows between them.
type TicTacToe struct {
	_ struct{} `meta:"name:tic-tac-toe,version:v1.0.0"`

	// Board positions (1 = available, 0 = taken)
	P00 dsl.TokenState `meta:"initial:1"`
	P01 dsl.TokenState `meta:"initial:1"`
	P02 dsl.TokenState `meta:"initial:1"`
	P10 dsl.TokenState `meta:"initial:1"`
	P11 dsl.TokenState `meta:"initial:1"`
	P12 dsl.TokenState `meta:"initial:1"`
	P20 dsl.TokenState `meta:"initial:1"`
	P21 dsl.TokenState `meta:"initial:1"`
	P22 dsl.TokenState `meta:"initial:1"`

	// X move history (0 = not played, 1 = X played here)
	X00 dsl.TokenState `meta:"initial:0"`
	X01 dsl.TokenState `meta:"initial:0"`
	X02 dsl.TokenState `meta:"initial:0"`
	X10 dsl.TokenState `meta:"initial:0"`
	X11 dsl.TokenState `meta:"initial:0"`
	X12 dsl.TokenState `meta:"initial:0"`
	X20 dsl.TokenState `meta:"initial:0"`
	X21 dsl.TokenState `meta:"initial:0"`
	X22 dsl.TokenState `meta:"initial:0"`

	// O move history (0 = not played, 1 = O played here)
	O00 dsl.TokenState `meta:"initial:0"`
	O01 dsl.TokenState `meta:"initial:0"`
	O02 dsl.TokenState `meta:"initial:0"`
	O10 dsl.TokenState `meta:"initial:0"`
	O11 dsl.TokenState `meta:"initial:0"`
	O12 dsl.TokenState `meta:"initial:0"`
	O20 dsl.TokenState `meta:"initial:0"`
	O21 dsl.TokenState `meta:"initial:0"`
	O22 dsl.TokenState `meta:"initial:0"`

	// Turn control (0 = X's turn, 1 = O's turn)
	Next dsl.TokenState `meta:"initial:0"`

	// Win detection
	WinX dsl.TokenState `meta:"initial:0"`
	WinO dsl.TokenState `meta:"initial:0"`

	// X move actions
	PlayX00 dsl.Action `meta:""`
	PlayX01 dsl.Action `meta:""`
	PlayX02 dsl.Action `meta:""`
	PlayX10 dsl.Action `meta:""`
	PlayX11 dsl.Action `meta:""`
	PlayX12 dsl.Action `meta:""`
	PlayX20 dsl.Action `meta:""`
	PlayX21 dsl.Action `meta:""`
	PlayX22 dsl.Action `meta:""`

	// O move actions
	PlayO00 dsl.Action `meta:""`
	PlayO01 dsl.Action `meta:""`
	PlayO02 dsl.Action `meta:""`
	PlayO10 dsl.Action `meta:""`
	PlayO11 dsl.Action `meta:""`
	PlayO12 dsl.Action `meta:""`
	PlayO20 dsl.Action `meta:""`
	PlayO21 dsl.Action `meta:""`
	PlayO22 dsl.Action `meta:""`

	// X win detection (rows, columns, diagonals)
	XRow0 dsl.Action `meta:""` // X00, X01, X02
	XRow1 dsl.Action `meta:""` // X10, X11, X12
	XRow2 dsl.Action `meta:""` // X20, X21, X22
	XCol0 dsl.Action `meta:""` // X00, X10, X20
	XCol1 dsl.Action `meta:""` // X01, X11, X21
	XCol2 dsl.Action `meta:""` // X02, X12, X22
	XDg0  dsl.Action `meta:""` // X00, X11, X22
	XDg1  dsl.Action `meta:""` // X20, X11, X02

	// O win detection (rows, columns, diagonals)
	ORow0 dsl.Action `meta:""` // O00, O01, O02
	ORow1 dsl.Action `meta:""` // O10, O11, O12
	ORow2 dsl.Action `meta:""` // O20, O21, O22
	OCol0 dsl.Action `meta:""` // O00, O10, O20
	OCol1 dsl.Action `meta:""` // O01, O11, O21
	OCol2 dsl.Action `meta:""` // O02, O12, O22
	ODg0  dsl.Action `meta:""` // O00, O11, O22
	ODg1  dsl.Action `meta:""` // O20, O11, O02
}

// Flows defines the morphisms (arcs) of the game.
// This is where the categorical structure becomes clear:
// - Objects: Places (P00, X00, O00, WinX, etc.)
// - Morphisms: Flows between them via actions
func (TicTacToe) Flows() []dsl.Flow {
	return []dsl.Flow{
		// X moves: Position -> PlayX -> History + Next
		{From: "P00", To: "PlayX00"}, {From: "PlayX00", To: "X00"}, {From: "PlayX00", To: "Next"},
		{From: "P01", To: "PlayX01"}, {From: "PlayX01", To: "X01"}, {From: "PlayX01", To: "Next"},
		{From: "P02", To: "PlayX02"}, {From: "PlayX02", To: "X02"}, {From: "PlayX02", To: "Next"},
		{From: "P10", To: "PlayX10"}, {From: "PlayX10", To: "X10"}, {From: "PlayX10", To: "Next"},
		{From: "P11", To: "PlayX11"}, {From: "PlayX11", To: "X11"}, {From: "PlayX11", To: "Next"},
		{From: "P12", To: "PlayX12"}, {From: "PlayX12", To: "X12"}, {From: "PlayX12", To: "Next"},
		{From: "P20", To: "PlayX20"}, {From: "PlayX20", To: "X20"}, {From: "PlayX20", To: "Next"},
		{From: "P21", To: "PlayX21"}, {From: "PlayX21", To: "X21"}, {From: "PlayX21", To: "Next"},
		{From: "P22", To: "PlayX22"}, {From: "PlayX22", To: "X22"}, {From: "PlayX22", To: "Next"},

		// O moves: Next + Position -> PlayO -> History
		{From: "Next", To: "PlayO00"}, {From: "P00", To: "PlayO00"}, {From: "PlayO00", To: "O00"},
		{From: "Next", To: "PlayO01"}, {From: "P01", To: "PlayO01"}, {From: "PlayO01", To: "O01"},
		{From: "Next", To: "PlayO02"}, {From: "P02", To: "PlayO02"}, {From: "PlayO02", To: "O02"},
		{From: "Next", To: "PlayO10"}, {From: "P10", To: "PlayO10"}, {From: "PlayO10", To: "O10"},
		{From: "Next", To: "PlayO11"}, {From: "P11", To: "PlayO11"}, {From: "PlayO11", To: "O11"},
		{From: "Next", To: "PlayO12"}, {From: "P12", To: "PlayO12"}, {From: "PlayO12", To: "O12"},
		{From: "Next", To: "PlayO20"}, {From: "P20", To: "PlayO20"}, {From: "PlayO20", To: "O20"},
		{From: "Next", To: "PlayO21"}, {From: "P21", To: "PlayO21"}, {From: "PlayO21", To: "O21"},
		{From: "Next", To: "PlayO22"}, {From: "P22", To: "PlayO22"}, {From: "PlayO22", To: "O22"},

		// X win detection: 3-in-a-row -> WinX
		{From: "X00", To: "XRow0"}, {From: "X01", To: "XRow0"}, {From: "X02", To: "XRow0"}, {From: "XRow0", To: "WinX"},
		{From: "X10", To: "XRow1"}, {From: "X11", To: "XRow1"}, {From: "X12", To: "XRow1"}, {From: "XRow1", To: "WinX"},
		{From: "X20", To: "XRow2"}, {From: "X21", To: "XRow2"}, {From: "X22", To: "XRow2"}, {From: "XRow2", To: "WinX"},
		{From: "X00", To: "XCol0"}, {From: "X10", To: "XCol0"}, {From: "X20", To: "XCol0"}, {From: "XCol0", To: "WinX"},
		{From: "X01", To: "XCol1"}, {From: "X11", To: "XCol1"}, {From: "X21", To: "XCol1"}, {From: "XCol1", To: "WinX"},
		{From: "X02", To: "XCol2"}, {From: "X12", To: "XCol2"}, {From: "X22", To: "XCol2"}, {From: "XCol2", To: "WinX"},
		{From: "X00", To: "XDg0"}, {From: "X11", To: "XDg0"}, {From: "X22", To: "XDg0"}, {From: "XDg0", To: "WinX"},
		{From: "X20", To: "XDg1"}, {From: "X11", To: "XDg1"}, {From: "X02", To: "XDg1"}, {From: "XDg1", To: "WinX"},

		// O win detection: 3-in-a-row -> WinO
		{From: "O00", To: "ORow0"}, {From: "O01", To: "ORow0"}, {From: "O02", To: "ORow0"}, {From: "ORow0", To: "WinO"},
		{From: "O10", To: "ORow1"}, {From: "O11", To: "ORow1"}, {From: "O12", To: "ORow1"}, {From: "ORow1", To: "WinO"},
		{From: "O20", To: "ORow2"}, {From: "O21", To: "ORow2"}, {From: "O22", To: "ORow2"}, {From: "ORow2", To: "WinO"},
		{From: "O00", To: "OCol0"}, {From: "O10", To: "OCol0"}, {From: "O20", To: "OCol0"}, {From: "OCol0", To: "WinO"},
		{From: "O01", To: "OCol1"}, {From: "O11", To: "OCol1"}, {From: "O21", To: "OCol1"}, {From: "OCol1", To: "WinO"},
		{From: "O02", To: "OCol2"}, {From: "O12", To: "OCol2"}, {From: "O22", To: "OCol2"}, {From: "OCol2", To: "WinO"},
		{From: "O00", To: "ODg0"}, {From: "O11", To: "ODg0"}, {From: "O22", To: "ODg0"}, {From: "ODg0", To: "WinO"},
		{From: "O20", To: "ODg1"}, {From: "O11", To: "ODg1"}, {From: "O02", To: "ODg1"}, {From: "ODg1", To: "WinO"},
	}
}

// Constraints defines the invariants of the game.
func (TicTacToe) Constraints() []dsl.Invariant {
	return []dsl.Invariant{
		// Total squares always 9 (conservation)
		{ID: "board_size", Expr: "P00+P01+P02+P10+P11+P12+P20+P21+P22+X00+X01+X02+X10+X11+X12+X20+X21+X22+O00+O01+O02+O10+O11+O12+O20+O21+O22 == 9"},
		// At most one winner
		{ID: "one_winner", Expr: "WinX + WinO <= 1"},
	}
}

// Schema returns the metamodel schema for Tic-tac-toe.
func Schema() *dsl.SchemaNode {
	node, _ := dsl.ASTFromStruct(TicTacToe{})
	return node
}
