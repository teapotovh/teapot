# MODULES := $(dir $(wildcard **/*/Makefile))
#
# .PHONY: all $(MODULES) run
#
# # $(MODULES):
# # 	$(MAKE) -C $* run
#
#
# $(MODULES):
# 	$(MAKE) -C $@
#
# %/run: %
# 	$(MAKE) -C $< run

MODULES := $(patsubst %/,%,$(dir $(wildcard cmd/*/Makefile)))
MAKECMDGOALS := proto

.PHONY: all $(MODULES)

proto:
	$(MAKE) -C proto all

$(MODULES):
	$(MAKE) -C $@ $(filter-out $(MODULES),$(MAKECMDGOALS)) ROOT=$(PWD)
