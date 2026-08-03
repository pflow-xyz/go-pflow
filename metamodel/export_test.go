package metamodel

// Fixtures shared with the external metamodel_test package.
//
// The tests that analyse a composed net now go through metamodel/metapetri,
// which imports metamodel — so they had to move to package metamodel_test to
// avoid an import cycle. They still want the ch04 fixtures defined alongside
// the internal composition tests, and these aliases are how an external test
// file reaches them. Test-only: this file is never part of the built package.
var (
	MustFlatten           = mustFlatten
	OrdersNet             = ordersNet
	InventoryNet          = inventoryNet
	OrdersInventoryBundle = ordersInventoryBundle
)
