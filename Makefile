.PHONY: build web-assets install install-serve uninstall-serve mac install-mac dev dev-serve dev-web test clean

# driver — build · install · install-serve (launchd) · uninstall-serve · mac ·
# install-mac · dev · test · clean. Every name and default below comes from
# docs/DESIGN.md "Names, paths, knobs"; keep the two in step.
#
# Knobs resolve in one order everywhere: the make line or the environment
# (`make install DRIVER_LABEL=x`), else the first KEY= line of ~/.driver/env,
# else the default. The env file is the one place both a hand-typed `make
# install` and `driver serve` read, so a value written there is the one both
# agree on.

# ~/.driver. DRIVER_HOME is environment/make-line only: the env file lives
# inside it, so it cannot come from the file.
DRIVER_HOME ?= $(HOME)/.driver
DRIVER_ENV_FILE := $(DRIVER_HOME)/env
# driver_env_value reads ONE key's value out of that file the way the daemon's
# env loader does: a UTF-8 BOM at the start of the file and a trailing CR (a
# CRLF file) are ignored, key and value are trimmed, the FIRST matching line
# wins, and quotes are kept verbatim — `unquote` strips one matching pair
# where a knob wants that. $(call driver_env_value,KEY).
driver_env_value = $(strip $(shell LC_ALL=C awk 'NR==1{sub(/^\357\273\277/,"")} {sub(/\r$$/,"")} /^[ \t]*$(1)[ \t]*=/{v=$$0; sub(/^[^=]*=[ \t]*/,"",v); sub(/[ \t]+$$/,"",v); print v; exit}' '$(DRIVER_ENV_FILE)' 2>/dev/null))
unquote = $(patsubst "%",%,$(patsubst '%',%,$(strip $(1))))

# launchd label of the `driver serve` job: names the plist file, its Label key
# and every launchctl target below. A set-but-empty value (environment or
# make line) would name a plist called ".plist" and target gui/<uid>/, so it
# reads as unset.
DRIVER_ENV_LABEL := $(call unquote,$(call driver_env_value,DRIVER_LABEL))
ifneq ($(DRIVER_ENV_LABEL),)
DRIVER_LABEL ?= $(DRIVER_ENV_LABEL)
endif
DRIVER_LABEL ?= dev.notttired.driver
override DRIVER_LABEL := $(call unquote,$(DRIVER_LABEL))
ifeq ($(DRIVER_LABEL),)
override DRIVER_LABEL := dev.notttired.driver
endif
PLIST := $(HOME)/Library/LaunchAgents/$(DRIVER_LABEL).plist

# Where the binary is INSTALLED — the `driver` file and the program the
# launchd plist runs. ~/.local/bin unless the knob says otherwise; the knob
# exists for a measured reason: an endpoint-security agent in lockdown mode
# (Santa on a managed Mac) lets an unsigned, locally built binary run only
# from a sanctioned tree — the user's source checkouts — and kills it
# everywhere else, so an install under ~/.local/bin comes up as a launchd job
# dying on every exec (`Killed: 9`) while the identical bytes in the checkout
# run. A leading ~/ is the home directory, a trailing slash is dropped; a
# relative path cannot name a launchd program, so it is refused with a note
# and falls through to the default.
DRIVER_ENV_BIN_DIR := $(call driver_env_value,DRIVER_BIN_DIR)
ifneq ($(DRIVER_ENV_BIN_DIR),)
DRIVER_BIN_DIR ?= $(DRIVER_ENV_BIN_DIR)
endif
DRIVER_BIN_DIR ?= $(HOME)/.local/bin
DRIVER_BIN_DIR_RESOLVED := $(patsubst %/,%,$(patsubst ~/%,$(HOME)/%,$(call unquote,$(DRIVER_BIN_DIR))))
ifeq ($(filter /%,$(DRIVER_BIN_DIR_RESOLVED)),)
ifneq ($(DRIVER_BIN_DIR_RESOLVED),)
$(info note: ignoring DRIVER_BIN_DIR=$(DRIVER_BIN_DIR) (origin: $(origin DRIVER_BIN_DIR); "file" means ~/.driver/env) — it must be an absolute path; using $(HOME)/.local/bin)
endif
DRIVER_BIN_DIR_RESOLVED := $(HOME)/.local/bin
endif
override DRIVER_BIN_DIR := $(DRIVER_BIN_DIR_RESOLVED)

# Where the macOS board app is INSTALLED by install-mac:
# $(DRIVER_APP_DIR)/Driver.app. /Applications unless the knob says otherwise,
# resolved by DRIVER_BIN_DIR's rules and for its reason: the agent that judges
# the driver binary by its path judges an app bundle's executable the same
# way, so an ad-hoc-signed Driver.app may have to live under the sanctioned
# tree too (`open <dir>/Driver.app` launches it once; Spotlight and the Dock
# know it from then on).
DRIVER_ENV_APP_DIR := $(call driver_env_value,DRIVER_APP_DIR)
ifneq ($(DRIVER_ENV_APP_DIR),)
DRIVER_APP_DIR ?= $(DRIVER_ENV_APP_DIR)
endif
DRIVER_APP_DIR ?= /Applications
DRIVER_APP_DIR_RESOLVED := $(patsubst %/,%,$(patsubst ~/%,$(HOME)/%,$(call unquote,$(DRIVER_APP_DIR))))
ifeq ($(filter /%,$(DRIVER_APP_DIR_RESOLVED)),)
ifneq ($(DRIVER_APP_DIR_RESOLVED),)
$(info note: ignoring DRIVER_APP_DIR=$(DRIVER_APP_DIR) (origin: $(origin DRIVER_APP_DIR); "file" means ~/.driver/env) — it must be an absolute path; using /Applications)
endif
DRIVER_APP_DIR_RESOLVED := /Applications
endif
override DRIVER_APP_DIR := $(DRIVER_APP_DIR_RESOLVED)

# How the app is BUILT: `xcode` — xcodegen + xcodebuild from macos/project.yml,
# which needs Xcode — or `clt`, macos/build-without-xcode.sh: the Command
# Line Tools' swiftc alone with an ad-hoc signature, for a Mac with no Xcode
# (the script's header says what it reproduces of the xcodebuild path). The
# default follows the ACTIVE developer dir: Xcode's carries usr/bin/xcodebuild
# and the Command Line Tools' does not (/usr/bin/xcodebuild is a shim that
# forwards to the active dir and errors out under the tools), so the probe
# answers exactly "would the bare `xcodebuild` the xcode recipes call work
# here". The knob — make line, environment, or a DRIVER_MAC_BUILDER= line of
# ~/.driver/env — forces either; any other value is refused with a note and
# the probe decides.
DRIVER_MAC_BUILDER_PROBED := $(shell d=$$(xcode-select -p 2>/dev/null) && test -x "$$d/usr/bin/xcodebuild" && echo xcode || echo clt)
DRIVER_ENV_MAC_BUILDER := $(call unquote,$(call driver_env_value,DRIVER_MAC_BUILDER))
ifneq ($(DRIVER_ENV_MAC_BUILDER),)
DRIVER_MAC_BUILDER ?= $(DRIVER_ENV_MAC_BUILDER)
endif
DRIVER_MAC_BUILDER ?= $(DRIVER_MAC_BUILDER_PROBED)
ifeq ($(and $(filter 1,$(words $(DRIVER_MAC_BUILDER))),$(filter xcode clt,$(DRIVER_MAC_BUILDER))),)
ifneq ($(strip $(DRIVER_MAC_BUILDER)),)
$(info note: ignoring DRIVER_MAC_BUILDER=$(DRIVER_MAC_BUILDER) (origin: $(origin DRIVER_MAC_BUILDER); "file" means ~/.driver/env) — it must be xcode or clt; using $(DRIVER_MAC_BUILDER_PROBED), what this Mac's developer dir says)
endif
override DRIVER_MAC_BUILDER := $(DRIVER_MAC_BUILDER_PROBED)
else
override DRIVER_MAC_BUILDER := $(strip $(DRIVER_MAC_BUILDER))
endif

# The PATH the launchd job runs with. A plist PATH REPLACES launchd's own
# (/usr/bin:/bin:/usr/sbin:/sbin) rather than extending it, and launchd jobs
# inherit none of the login shell's PATH — so without this `driver serve`
# finds neither pi nor tmux nor gh nor node, every pi pane inheriting the
# daemon's PATH wholesale. The PATH= line of ~/.driver/env comes first when
# there is one (`install` writes the installing shell's PATH there on a first
# install; a hand-written line may be just the one dir that holds pi), and
# the plain default follows it, so the string always ends in the system dirs
# osascript's friends live in. A leading ~/ in an entry is the home directory
# (launchd expands nothing). A non-default DRIVER_BIN_DIR is prepended so a
# bare `driver` resolves to the real binary rather than its ~/.local/bin link.
DRIVER_ENV_PATH_RAW := $(call unquote,$(call driver_env_value,PATH))
DRIVER_ENV_PATH := $(if $(DRIVER_ENV_PATH_RAW),$(patsubst :%,%,$(subst :~/,:$(HOME)/,:$(DRIVER_ENV_PATH_RAW))))
DRIVER_DEFAULT_PATH := $(if $(filter-out $(HOME)/.local/bin,$(DRIVER_BIN_DIR)),$(DRIVER_BIN_DIR):)$(HOME)/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin
DRIVER_PLIST_PATH := $(if $(DRIVER_ENV_PATH),$(DRIVER_ENV_PATH):)$(DRIVER_DEFAULT_PATH)
# DRIVER_HOME rides into the plist only when it is not the default, so the
# daemon and this Makefile agree on where the env file and the log are
DRIVER_PLIST_HOME_LINE := $(if $(filter-out $(HOME)/.driver,$(DRIVER_HOME)),'    <key>DRIVER_HOME</key><string>$(DRIVER_HOME)</string>')
# only for the URL the recipes print; the daemon reads the same line itself
DRIVER_PORT := $(or $(call unquote,$(call driver_env_value,DRIVER_PORT)),7781)

# the name the board shows as the operator. `install` writes it into a FRESH
# ~/.driver/env as DRIVER_OPERATOR; an existing file is never touched.
# Defaults to the account's full name.
OPERATOR ?= $(shell id -F 2>/dev/null)

# stamped into the binary (`driver version`, /api/health): the described
# commit, `-dirty` when the tree has edits, `dev` before the first commit.
VERSION := $(shell git describe --always --dirty 2>/dev/null || echo dev)

# ---------------------------------------------------------------------------

# web-assets builds the React board into web/_ui/dist, which web/embed.go
# pulls into the binary with `//go:embed all:_ui/dist`. A PREREQUISITE of
# build, and that edge is what makes the ordering correct: go:embed reads the
# directory from disk at compile time with no dependency tracking of its own,
# so a bare `go build` embeds whatever stale bundle sits there — or fails
# outright on a clean clone, since go:embed cannot embed an empty directory
# and dist/ is gitignored. `npm ci`, not `npm install`: it installs exactly
# the lockfile and fails instead of silently resolving a new version.
web-assets:
	cd web/_ui && npm ci --no-audit --no-fund && npm run build

build: web-assets
	go build -ldflags "-X main.version=$(VERSION)" -o driver ./cmd/driver

# install: build, put the binary in place, seed ~/.driver/env on a first
# install, then (re)load the launchd job on the fresh binary. install-serve
# runs LAST, from the recipe rather than as a prerequisite, so the job is
# never bootstrapped against a binary that is not there yet.
install: build
	mkdir -p $(DRIVER_BIN_DIR)
	# cp to a sibling then mv, never cp over: overwriting a running binary in
	# place gets the live daemon SIGKILLed by macOS code signing; the rename
	# swaps the directory entry and leaves the running process on its old
	# inode.
	cp driver $(DRIVER_BIN_DIR)/driver.tmp
	mv -f $(DRIVER_BIN_DIR)/driver.tmp $(DRIVER_BIN_DIR)/driver
ifneq ($(filter-out $(HOME)/.local/bin,$(DRIVER_BIN_DIR)),)
	# The binary lives outside ~/.local/bin (DRIVER_BIN_DIR, preamble): leave
	# `driver` there as a symlink to it, so a PATH carrying ~/.local/bin still
	# finds it. -f replaces whatever is at the path — a regular file left by an
	# earlier default install included, and unlinking never touches the inode
	# a running daemon holds open; -n keeps an existing link from being
	# followed into its target.
	mkdir -p $(HOME)/.local/bin
	ln -sfn $(DRIVER_BIN_DIR)/driver $(HOME)/.local/bin/driver
endif
	@echo "installed driver $(VERSION) → $(DRIVER_BIN_DIR)/driver"
	# First install only: a commented env file naming the operator and
	# carrying this shell's PATH (see DRIVER_PLIST_PATH above for why). The
	# install dirs are written as live lines when this make was given
	# non-default ones (`make install DRIVER_BIN_DIR=…`), so the knob sticks
	# for every later bare `make install`; otherwise as commented defaults. An
	# existing file — edited by hand or not — is never rewritten. A failed
	# write FAILS the recipe: a daemon with no PATH line finds no pi, and
	# "wrote" over a write that did not happen would hide why.
	@env='$(DRIVER_ENV_FILE)'; op='$(subst ','\'',$(OPERATOR))'; \
	if [ -e "$$env" ]; then echo "$$env exists — left as is"; exit 0; fi; \
	mkdir -p '$(DRIVER_HOME)' || { echo "error: could not create $(DRIVER_HOME)" >&2; exit 1; }; \
	printf '%s\n' \
	  '# ~/.driver/env — KEY=VALUE lines; # comments on their own lines. Read by' \
	  '# `driver serve` at start and by make (install, install-serve, mac). After' \
	  '# an edit, `make install-serve` reloads the launchd job with the new values.' \
	  '# Every knob: docs/DESIGN.md "Names, paths, knobs".' \
	  '' \
	  '# the name the board shows as the operator (else $$USER)' \
	  "DRIVER_OPERATOR=$$op" \
	  '' \
	  '# launchd jobs inherit none of the login shell PATH: this is the PATH the' \
	  '# daemon — and every pi it starts — runs with. Captured from the shell that' \
	  '# ran `make install`; pi, tmux, gh and node must resolve on it.' \
	  "PATH=$$PATH" \
	  '' \
	  '# where new jobs start (else $$HOME)' \
	  '#DRIVER_DEFAULT_DIR=~/src' \
	  '# the pi binary (else `pi` on PATH) and the extra flags it gets — default' \
	  '# -a (trust project files); an empty DRIVER_PI_FLAGS= passes none' \
	  '#DRIVER_PI=/path/to/pi' \
	  '#DRIVER_PI_FLAGS=-a' \
	  '# macOS notifications when a job blocks or its pi exits' \
	  '#DRIVER_NOTIFY=off' \
	  '# loopback port · launchd label · install dirs · app builder (clt | xcode)' \
	  '#DRIVER_PORT=7781' \
	  '#DRIVER_LABEL=dev.notttired.driver' \
	  $(if $(filter-out $(HOME)/.local/bin,$(DRIVER_BIN_DIR)),'DRIVER_BIN_DIR=$(DRIVER_BIN_DIR)','#DRIVER_BIN_DIR=~/.local/bin') \
	  $(if $(filter-out /Applications,$(DRIVER_APP_DIR)),'DRIVER_APP_DIR=$(DRIVER_APP_DIR)','#DRIVER_APP_DIR=/Applications') \
	  '#DRIVER_MAC_BUILDER=clt' \
	  > "$$env.tmp" && mv -f "$$env.tmp" "$$env" \
	  || { echo "error: could not write $$env" >&2; exit 1; }; \
	echo "wrote $$env (operator: $${op:-<none — set DRIVER_OPERATOR>}; PATH captured from this shell)"
	$(MAKE) install-serve

# The plist is generated, not checked in: it embeds $(HOME) paths and the env
# file's PATH. RunAtLoad + KeepAlive make launchd start the daemon at login
# and restart it when it dies; ThrottleInterval keeps a crash loop (a port
# already taken) from spinning. Then bootout + bootstrap — the only pair that
# makes launchd RE-READ the file (kickstart relaunches from its cached
# definition) — and kickstart -k so the job is up on the fresh binary now.
# bootout fails harmlessly when nothing is loaded yet.
install-serve:
	mkdir -p $(HOME)/Library/LaunchAgents $(DRIVER_HOME)
	@printf '%s\n' \
	  '<?xml version="1.0" encoding="UTF-8"?>' \
	  '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
	  '<plist version="1.0"><dict>' \
	  '  <key>Label</key><string>$(DRIVER_LABEL)</string>' \
	  '  <key>ProgramArguments</key><array>' \
	  '    <string>$(DRIVER_BIN_DIR)/driver</string>' \
	  '    <string>serve</string>' \
	  '  </array>' \
	  '  <key>RunAtLoad</key><true/>' \
	  '  <key>KeepAlive</key><true/>' \
	  '  <key>ThrottleInterval</key><integer>30</integer>' \
	  '  <key>StandardOutPath</key><string>$(DRIVER_HOME)/driver.log</string>' \
	  '  <key>StandardErrorPath</key><string>$(DRIVER_HOME)/driver.log</string>' \
	  '  <key>EnvironmentVariables</key><dict>' \
	  '    <key>PATH</key><string>$(DRIVER_PLIST_PATH)</string>' \
	  '    <key>LANG</key><string>en_US.UTF-8</string>' $(DRIVER_PLIST_HOME_LINE) \
	  '  </dict>' \
	  '</dict></plist>' > $(PLIST).new
	@plutil -lint $(PLIST).new >/dev/null && mv -f $(PLIST).new $(PLIST) && echo "wrote $(PLIST)"
	@uid=$$(id -u); \
	launchctl bootout gui/$$uid/$(DRIVER_LABEL) 2>/dev/null || true; \
	launchctl bootstrap gui/$$uid $(PLIST) \
	  || { echo "error: launchctl bootstrap failed — the plist is written; 'launchctl bootstrap gui/$$uid $(PLIST)' by hand, or log out and in" >&2; exit 1; }; \
	launchctl kickstart -k gui/$$uid/$(DRIVER_LABEL) \
	  || echo "warning: launchctl kickstart failed — 'launchctl print gui/$$uid/$(DRIVER_LABEL)' says why"; \
	echo "driver serve is running under launchd ($(DRIVER_LABEL)) — http://127.0.0.1:$(DRIVER_PORT)  (log: $(DRIVER_HOME)/driver.log)"

# Unload the job and remove the plist. The binary, ~/.driver and its database
# stay.
uninstall-serve:
	-launchctl bootout gui/$$(id -u)/$(DRIVER_LABEL) 2>/dev/null
	rm -f $(PLIST)
	@echo "driver serve unloaded — $(PLIST) removed; $(DRIVER_BIN_DIR)/driver and $(DRIVER_HOME) are still there"

# Compile check of the mac app; nothing is installed. Which builder, see
# DRIVER_MAC_BUILDER above.
mac:
ifeq ($(DRIVER_MAC_BUILDER),xcode)
	@command -v xcodegen >/dev/null 2>&1 || { echo "xcodegen not found — brew install xcodegen"; exit 1; }
	xcodegen generate --spec macos/project.yml
	xcodebuild build -project macos/Driver.xcodeproj -scheme Driver -destination 'platform=macOS'
else
	@set -e; \
	dd=$$(mktemp -d /tmp/driver-mac-build.XXXXXX); \
	trap 'rm -rf "$$dd"' EXIT; \
	sh macos/build-without-xcode.sh "$$dd"; \
	echo "compiled Driver.app with the Command Line Tools (removed again — make install-mac installs it)"
endif

# The swap install-mac performs, written once for both builders. Same
# never-write-over-a-live-bundle discipline as `install`, for the same macOS
# reason: modifying a running app's files invalidates its code signature and
# gets the process SIGKILLed. A bundle is a DIRECTORY, so the single `mv -f`
# that works for the binary does not apply — `mv` onto an existing directory
# moves the source INSIDE it. The dance is instead: stage a full copy under a
# dot-name in the app dir (same filesystem, so the renames are cheap and
# atomic), rename any existing Driver.app OUT of the way, rename the staged
# copy IN, then unlink the old one — which a running app survives on its open
# inodes. Quitting the app first is still how you get the new build; this
# recipe deliberately does not kill it.
INSTALL_APP = app="$$1"; dir='$(DRIVER_APP_DIR)'; \
	[ -d "$$app" ] || { echo "build produced no Driver.app at $$app" >&2; exit 1; }; \
	mkdir -p "$$dir"; \
	rm -rf "$$dir/.Driver.app.new"; \
	cp -R "$$app" "$$dir/.Driver.app.new"; \
	if [ -d "$$dir/Driver.app" ]; then rm -rf "$$dir/.Driver.app.old"; mv "$$dir/Driver.app" "$$dir/.Driver.app.old"; fi; \
	mv "$$dir/.Driver.app.new" "$$dir/Driver.app" \
	  || { mv "$$dir/.Driver.app.old" "$$dir/Driver.app" 2>/dev/null; exit 1; }; \
	rm -rf "$$dir/.Driver.app.old"; \
	echo "installed Driver.app → $$dir (quit and relaunch to pick it up)";

# Build Release and install into $(DRIVER_APP_DIR). Under the xcode builder
# xcodegen rewrites macos/Generated-Info.plist from project.yml's info block;
# a diff there afterwards means the two drifted — commit the regenerated one.
install-mac:
ifeq ($(DRIVER_MAC_BUILDER),xcode)
	@command -v xcodegen >/dev/null 2>&1 || { echo "xcodegen not found — brew install xcodegen"; exit 1; }
	@set -e; \
	dd=$$(mktemp -d /tmp/driver-mac-build.XXXXXX); \
	trap 'rm -rf "$$dd"' EXIT; \
	xcodegen generate --spec macos/project.yml; \
	xcodebuild build -project macos/Driver.xcodeproj -scheme Driver \
	  -configuration Release -destination 'platform=macOS' -derivedDataPath "$$dd"; \
	install_app() { $(INSTALL_APP) }; \
	install_app "$$dd/Build/Products/Release/Driver.app"
else
	@set -e; \
	dd=$$(mktemp -d /tmp/driver-mac-build.XXXXXX); \
	trap 'rm -rf "$$dd"' EXIT; \
	sh macos/build-without-xcode.sh "$$dd"; \
	install_app() { $(INSTALL_APP) }; \
	install_app "$$dd/Driver.app"
endif

# Development: the daemon from source and Vite with hot reload. `make dev`
# runs both under one make (-j2 — Ctrl-C stops both, output interleaves); or
# `make dev-serve` and `make dev-web` in two terminals to keep them apart.
# The launchd daemon holds :7781, so either `make uninstall-serve` first or
# move the dev daemon: `make dev-serve SERVE_ARGS="-p 7782"`.
dev:
	$(MAKE) -j2 dev-serve dev-web

dev-serve:
	@test -d web/_ui/dist || $(MAKE) web-assets
	go run ./cmd/driver serve $(SERVE_ARGS)

dev-web:
	cd web/_ui && npm run dev

# go:embed needs web/_ui/dist to exist even for `go test`, hence the check.
# `--if-present` keeps a board without a test script from failing the Go run.
test:
	@test -d web/_ui/dist || $(MAKE) web-assets
	go test ./...
	cd web/_ui && npm test --if-present

# node_modules is deliberately NOT removed: it is expensive to rebuild and
# `npm ci` replaces it wholesale anyway. dist IS removed — it is the embed
# target, so a stale one is the thing most worth being able to clear; after a
# clean, a bare `go build` fails until `make build` regenerates it.
clean:
	rm -f driver
	rm -rf web/_ui/dist macos/build macos/Driver.xcodeproj
