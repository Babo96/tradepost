WOW_DIR ?= /games/World of Warcraft
FOREVER := $(WOW_DIR)/_forever_
ADDONS := $(FOREVER)/Interface/AddOns
PYTHON ?= python3

PAYLOAD ?= TEST123
KEYS ?= 4
MODE ?= click
SLOT ?= A
CHUNK_SIZE ?= 200
SEQ ?= 1
KEY ?= CTRL-ALT-SHIFT-F1
COMMAND ?= ACTIONBUTTON1
ACCOUNT ?=
ACCOUNT_ARG := $(if $(ACCOUNT),--account $(ACCOUNT),)

PROBE_ADDONS := BaboForeverProbe BFProbeDataA BFProbeDataB BFProbeDataC BFProbeDataD

.PHONY: help addon-install addon-uninstall probe-install probe-uninstall probe-show probe-bind probe-ring probe-meta probe-set probe-clear probe-reset probe-restore lint test check

help:
	@echo "Babo Forever"
	@echo
	@echo "  make addon-install          install BaboForever + BaboForever_Data"
	@echo
	@echo "  make addon-uninstall        remove the real addon"
	@echo
	@echo "  make probe-install          copy the probe addons into $(ADDONS)"
	@echo "  make probe-uninstall        remove the probe addons"
	@echo "  make probe-show             show the current BFD1 mailbox lines"
	@echo "  make probe-bind             write PAYLOAD to the bindings mailbox"
	@echo "  make probe-ring             rewrite LoadOnDemand slot SLOT with PAYLOAD"
	@echo "  make probe-meta             rewrite X-BF-Data in slot SLOT with PAYLOAD"
	@echo "  make probe-set              write a known-valid binding (KEY=COMMAND)"
	@echo "  make probe-clear            remove BFD1 lines from the bindings file"
	@echo "  make probe-reset            remove all probe/reserved binding lines"
	@echo "  make probe-restore          restore the newest bindings backup"
	@echo "  make lint                   syntax-check Lua and Python"
	@echo "  make test                   run the Python unit tests"
	@echo "  make check                  lint + test"
	@echo
	@echo "Variables: WOW_DIR=$(WOW_DIR) ACCOUNT=$(ACCOUNT) PAYLOAD=$(PAYLOAD) KEYS=$(KEYS) MODE=$(MODE) SLOT=$(SLOT) KEY=$(KEY) COMMAND=$(COMMAND)"

addon-install:
	@test -d "$(FOREVER)" || (echo "WoW Forever not found at $(FOREVER); set WOW_DIR" >&2; exit 1)
	@mkdir -p "$(ADDONS)"
	@rm -rf "$(ADDONS)/BaboForever" "$(ADDONS)/BaboForever_Data"
	@cp -R addon/BaboForever "$(ADDONS)/BaboForever"
	@cp -R data-addon/BaboForever_Data "$(ADDONS)/BaboForever_Data"
	@echo "Installed BaboForever into $(ADDONS)"
	@echo "Restart WoW, then /reload and /bf status."

addon-uninstall:
	@rm -rf "$(ADDONS)/BaboForever" "$(ADDONS)/BaboForever_Data"
	@echo "Removed BaboForever"

probe-install:
	@test -d "$(FOREVER)" || (echo "WoW Forever not found at $(FOREVER); set WOW_DIR" >&2; exit 1)
	@mkdir -p "$(ADDONS)"
	@for addon in $(PROBE_ADDONS); do rm -rf "$(ADDONS)/$$addon"; done
	@for addon in $(PROBE_ADDONS); do cp -R "probe/$$addon" "$(ADDONS)/"; done
	@echo "Installed probe addons into $(ADDONS)"
	@echo "Restart WoW (new addons are only noticed at startup), then /reload."

probe-uninstall:
	@for addon in $(PROBE_ADDONS); do rm -rf "$(ADDONS)/$$addon"; done
	@echo "Removed probe addons from $(ADDONS)"

probe-show:
	@$(PYTHON) tools/probe/rewrite_bindings.py --wow "$(WOW_DIR)" $(ACCOUNT_ARG) --show

probe-bind:
	@$(PYTHON) tools/probe/rewrite_bindings.py --wow "$(WOW_DIR)" $(ACCOUNT_ARG) --payload "$(PAYLOAD)" --keys $(KEYS) --mode $(MODE) --chunk-size $(CHUNK_SIZE) --seq $(SEQ) --write

probe-ring:
	@$(PYTHON) tools/probe/write_slot.py --wow "$(WOW_DIR)" --slot "$(SLOT)" --payload "$(PAYLOAD)" --write

probe-meta:
	@$(PYTHON) tools/probe/write_toc.py --wow "$(WOW_DIR)" --addon "BFProbeData$(SLOT)" --payload "$(PAYLOAD)" --write

probe-set:
	@$(PYTHON) tools/probe/set_binding.py --wow "$(WOW_DIR)" $(ACCOUNT_ARG) --key "$(KEY)" --command "$(COMMAND)" --write

probe-reset:
	@$(PYTHON) tools/probe/reset_probe.py --wow "$(WOW_DIR)" $(ACCOUNT_ARG) --write

probe-clear:
	@$(PYTHON) tools/probe/rewrite_bindings.py --wow "$(WOW_DIR)" $(ACCOUNT_ARG) --clear --write

probe-restore:
	@$(PYTHON) tools/probe/rewrite_bindings.py --wow "$(WOW_DIR)" $(ACCOUNT_ARG) --restore

lint:
	@$(PYTHON) -m py_compile tools/probe/*.py
	@if command -v luac5.1 >/dev/null 2>&1; then luac5.1 -p probe/*/*.lua addon/*/*.lua data-addon/*/*.lua; else luac -p probe/*/*.lua addon/*/*.lua data-addon/*/*.lua; fi
	@echo "syntax ok"

test:
	@$(PYTHON) -m unittest discover -s tools/probe -p 'test_*.py' -v

check: lint test
