.PHONY: all clean

PROTO_FILES := $(wildcard *.proto)
PROTO_STEMS := $(basename $(PROTO_FILES))

all: $(PROTO_STEMS) proto/

%/: 
	mkdir -p $@

define PROTO_RULE
$(1)/$(1)_vtproto.pb.go: $(1).proto | $(1)/
	protoc -I. --go-vtproto_out=. ./$(1).proto

$(1)/$(1).pb.go: $(1).proto | $(1)/
	protoc -I. --go_out=. ./$(1).proto

proto/$(1)_pb2.py: $(1).proto | proto/
	cd proto/ && python -m grpc_tools.protoc -I.. --python_out=. --pyi_out=. --grpc_python_out=. ../$(1).proto
	cd proto/ && sed -i 's/^import embed_pb2/import proto.embed_pb2/' $(1)_pb2_grpc.py

proto/$(1).py: proto/ proto/$(1)_pb2.py
	echo "from proto.$(1)_pb2 import *" > proto/$(1).py
	echo "from proto.$(1)_pb2_grpc import *" >> proto/$(1).py

$(1): $(1)/$(1).pb.go $(1)/$(1)_vtproto.pb.go proto/$(1)_pb2.py proto/$(1).py 
endef

$(foreach stem,$(PROTO_STEMS),$(eval $(call PROTO_RULE,$(stem))))

clean:
	rm -rf $(PROTO_STEMS) proto/
