/*
Package graphql provides a GraphQL server for Petri net models.

It automatically generates GraphQL schemas from Petri net definitions,
enabling zero-config API access to workflow models.

# Basic Usage

Create a server with one or more Petri net models:

	model := petri.NewPetriNet()
	model.AddPlace("pending", 1, 0, 0, 0, nil)
	model.AddPlace("approved", 0, 0, 100, 0, nil)
	model.AddTransition("approve", "", 50, 0, nil)
	model.AddArc("pending", "approve", 1, false)
	model.AddArc("approve", "approved", 1, false)

	server := graphql.NewServer(
		graphql.WithModel("approval", model, myStore),
		graphql.WithPlayground("/graphql/i"),
	)

	http.ListenAndServe(":8080", server.Mux())

# Generated Schema

For each model, the package generates:

  - Query type with instance and instances fields
  - Mutation type with create and one mutation per transition
  - Instance type with marking and enabled transitions
  - Input types for each transition

Example generated schema:

	type Query {
	  instance(id: ID!): Instance
	  instances(place: String, page: Int): InstanceList!
	}

	type Mutation {
	  create: Instance!
	  approve(input: ApproveInput!): TransitionResult!
	}

	type Instance {
	  id: ID!
	  version: Int!
	  marking: Marking!
	  enabledTransitions: [String!]!
	}

# Multi-Model Support

When multiple models are registered, the schema is unified with
namespaced types to avoid conflicts:

	server := graphql.NewServer(
		graphql.WithModel("order", orderModel, orderStore),
		graphql.WithModel("payment", paymentModel, paymentStore),
	)

This produces queries like orderInstance, paymentInstance and
mutations like order_create, payment_process.

# Introspection

The server supports full GraphQL introspection, enabling use with
standard GraphQL tools and IDEs.
*/
package graphql
