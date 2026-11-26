"Define linter aspects"

load("@rules_lint//lint:golangci_lint.bzl", "lint_golangci_lint_aspect")

golangci_lint = lint_golangci_lint_aspect(
    binary = Label("@rules_lint//lint:golangci_lint_bin"),
    config = Label("//:.golangci.yaml"),
    rule_kinds = [
        "go_library",
        "go_test",
    ],
)
