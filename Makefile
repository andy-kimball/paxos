# Allow setting of go build flags from the command line.
GOFLAGS :=
TAGS    :=

GO := @go
ROOT  := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
GOBIN := $(ROOT)/../../../../bin
GOSRC := $(ROOT)/../../../../src


# ================================================================================
# Tracing-related variables.
# ================================================================================

# Add TRACE=<level> to emit test tracing information.

# To create trace file that can be easily grep'd:
#   2> trace.txt && cat trace.txt | more

ifdef TRACE
	TRACEFLAG += -trace.level=$(TRACE)
endif


# ================================================================================
# Build-related make rules.
# ================================================================================

.PHONY: build
build: protos
build:
	$(GO) fmt > /dev/null
	$(GO) build $(GOFLAGS)

.PHONY: clean
clean: clean-protos
clean:
	$(GO) clean $(GOFLAGS) -i


# ================================================================================
# Testing-related make rules.
# ================================================================================

# Variables to be overridden on the command line, e.g.
#   make test TESTS=./cluster TESTRUN=Node
TESTS        := ./...
TESTTIMEOUT  := 10s
TESTFLAGS    :=
TESTRUN      := .

.PHONY: test
test: protos
test: TESTFLAGS += -v
test:
	$(GO) test $(GOFLAGS) -timeout $(TESTTIMEOUT) -run $(TESTRUN) $(TESTFLAGS) $(TESTS) $(TRACEFLAG) | \
		sed -E 's/=== RUN   //' | \
		sed -E 's/^--- (PASS|FAIL): .+ \((.+)\)/    \1    \2/' | \
		sed -E ''/^PASS/s//`printf "\033[92mPASS\033[0m"`/'' | \
		sed -E ''/FAIL/s//`printf "\033[91mFAIL\033[0m"`/''

.PHONY: testrace
testrace: TESTFLAGS += -race
testrace: test

.PHONY: cover
cover: TESTFLAGS += -coverprofile=coverage.out
cover: test
cover:
	$(GO) tool cover -html=coverage.out && rm coverage.out


# ================================================================================
# Protobuf-related make rules.
# ================================================================================

GOGOPROTO_ROOT := $(GOSRC)/github.com/gogo/protobuf
GOGOPROTO_PATH := $(GOGOPROTO_ROOT):$(GOGOPROTO_ROOT)/protobuf

# The shell command finds all non-vendor files ending with ".proto".
# Replace ".proto" with ".pb.go" and then "_test.pb.go" with ".pb_test.go"
# to get the GO_PROTOS list.
PROTOS    := $(shell find . -name '*.proto' -and -not -path './vendor/*')
GO_PROTOS := $(PROTOS:%.proto=%.pb.go)
GO_PROTOS := $(GO_PROTOS:%_test.pb.go=%.pb_test.go)

.PHONY: protos
protos: $(GO_PROTOS)

.PHONY: clean-protos
clean-protos:
	@find $(GO_PROTOS) 2> /dev/null | xargs rm -f

%.pb_test.go: %_test.pb.go
    # Rename foo_test.pb.go to foo.pb_test.go so that types in these files can't
    # be referenced in non-test builds.
	@mv $< $@

%.pb.go: %.proto
    # The $(<D) automatic variable gets the directory part of the prerequisite. Pass all
    # protos in the same directory to `protoc` so that imports get correctly resolved.
	@cd $(<D) && \
	  protoc -I=.:$(GOGOPROTO_PATH) --gofast_out=. *.proto
    # Disable package comment that interferes with godoc by inserting blank line after comment.
	@sed -i.tmp $$'s/^package /\\\npackage /g' $@
	@rm $@.tmp
    # Format the resulting file.
	@gofmt -s -w $@


# ================================================================================
# Lines of code make rules.
# ================================================================================

.PHONY: loc
loc:
	@find . -name '*.go' -and -not -name '*_test.go' -and -not -name '*.pb.go' -and -not -path './vendor/*' | \
	xargs wc -l
