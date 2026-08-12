# Code style. The Go files are gofmt'd; the shell files under lima/ and
# completions/ are shfmt'd, taking their 4-space indent from .editorconfig.
# Split per language so CI's two jobs can each run their half.

SHFMT ?= go run mvdan.cc/sh/v3/cmd/shfmt@latest
SHELL_DIRS := lima completions
# git ls-files, not `.`: it skips the untracked worktrees under .claude/.
GO_FILES = $$(git ls-files '*.go')

.PHONY: format format-go format-shell check-format check-format-go check-format-shell

format: format-go format-shell

format-go:
	gofmt -w $(GO_FILES)

format-shell:
	$(SHFMT) -w $(SHELL_DIRS)

check-format: check-format-go check-format-shell

check-format-go:
	@out=$$(gofmt -l $(GO_FILES)); test -z "$$out" || { echo "$$out"; exit 1; }

check-format-shell:
	$(SHFMT) -d $(SHELL_DIRS)
