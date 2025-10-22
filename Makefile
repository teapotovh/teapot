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

.PHONY: all $(MODULES) echo

echo:
	echo $(MODULES)

$(MODULES):
	$(MAKE) -C $@ $(filter-out $(MODULES),$(MAKECMDGOALS))

%: $(MODULES)
	@:
