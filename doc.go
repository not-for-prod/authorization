// Package authorization embeds the authentication engine in a host application.
// Hosts supply nested tenant/intent options with policy-bound drivers, storage, access rules, and
// optional session issuance. New constructs the application service; NewHTTPHandler
// optionally exposes the existing HTTP contract without starting a listener.
//
// Public aliases preserve the domain contracts and their methods without requiring
// consumers to import internal packages. Adapters must follow the documented
// transaction, proof, snapshot, and idempotent session-issuance contracts.
package authorization
