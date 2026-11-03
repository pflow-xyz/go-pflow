# composite goldens

`composite.json` is the reference answers of pflow-rs's `bundle_net`,
`bundle_prove`, `bundle_check_proof` and `bundle_parts`, copied byte for byte
from `crates/pflow-hosted/tests/fixtures/composite/composite.json`:

- pflow-rs commit: `16f69e0`
- sha256: `8bf9ee18f34bd1412403b67e101f53046c08a51348acdc23f4679bc3c8f7d934`

pflow-rs is the reference of these tools; this file is regenerated there
(`REGEN_COMPOSITE=1 cargo test -p pflow-hosted --test composite_goldens`), never
here and never to make a test pass.

Shape: `catalog` (the `bundle_parts` answer with no arguments) and `cases`, each
`{name, spec, bundle, net, prove, check_valid}` where `spec` is the
`bundle_parts` call that built `bundle`, `net` the `bundle_net` answer and
`prove` the `bundle_prove` record.

`golden_test.go` replays it. Held exactly (JSON-equal): the catalog; each
`bundle`; each `net` answer; and, of each `prove` record, `composite`
`{cid, places, transitions, arcs}`, `verdict`, each link's `{key, kind,
structural, glue, verdict, findings[].code, effect}`, each claim's `{id,
status, method}` and each law's `{part, id, declared, law, status,
holds_on_reachable, residual, lifted}` with whether it has `extended_by`.

Nothing is relaxed. (The task allowed comparing Farkas-ordered laws as a set,
and witnesses and `extended_by` by replay, if go-pflow's Farkas/BFS order
differed; it does not: the Go records equal pflow-rs's record for record, so
the whole `prove` answers are in fact identical, which the test does not need
to rely on.) The golden records also check here (`bundle_check_proof` valid on
every one), and `GO_RECORDS_OUT=path go test ./composite -run GoldenCases`
writes Go's own records in the golden's shape for pflow-rs's
`foreign_records_check`:

    GO_RECORDS_OUT=/tmp/go_records.json go test ./composite -run GoldenCases
    cd pflow-rs && FOREIGN_RECORDS=/tmp/go_records.json \
      cargo test -p pflow-hosted --test composite_goldens foreign_records_check

One difference from pflow-rs that the goldens do not see: `bundle_prove` here
refuses a flat net with a guard or a data place (pflow-rs analyses it with its
own guard-aware model and says `net_available: false`); go-pflow's
reachability does not evaluate guards, so an answer about such a net would be an
answer about another one.
