############################# Main targets #############################
# Install all tools and builds binaries.
install: update-tools proto bins

# Run all possible checks and tests.
all: proto check bins test

# Rebuild binaries.
bins: clean-bins proto temporal-server tctl temporal-cassandra-tool temporal-sql-tool temporal-canary

# Delete all build artefacts.
clean: clean-bins clean-proto clean-test-results

# Update proto submodule from remote and rebuild proto files.
update-proto: clean-proto update-proto-submodule protoc update-proto-go proto-mock gomodtidy
########################################################################

.PHONY: proto

##### Variables ######
ifndef GOOS
GOOS := $(shell go env GOOS)
endif

ifndef GOARCH
GOARCH := $(shell go env GOARCH)
endif

ifndef GOPATH
GOPATH := $(shell go env GOPATH)
endif

MODULE_ROOT := github.com/temporalio/temporal
BUILD := ./build
COLOR := "\e[1;36m%s\e[0m\n"

define NEWLINE


endef

TEST_TIMEOUT := 20m
TEST_ARG ?= -race -v -timeout $(TEST_TIMEOUT)

INTEG_TEST_ROOT        := ./host
INTEG_TEST_OUT_DIR     := host
INTEG_TEST_XDC_ROOT    := ./host/xdc
INTEG_TEST_XDC_OUT_DIR := hostxdc
INTEG_TEST_NDC_ROOT    := ./host/ndc
INTEG_TEST_NDC_OUT_DIR := hostndc

GO_BUILD_LDFLAGS_CMD      := $(abspath ./scripts/go-build-ldflags.sh)
GO_BUILD_LDFLAGS          := $(shell $(GO_BUILD_LDFLAGS_CMD) LDFLAG)

ifndef PERSISTENCE_TYPE
override PERSISTENCE_TYPE := cassandra
endif

ifndef TEST_RUN_COUNT
override TEST_RUN_COUNT := 1
endif

ifdef TEST_TAG
override TEST_TAG := -tags $(TEST_TAG)
endif

ALL_SRC         := $(shell find . -name "*.go" | grep -v -e ".gen")
TEST_DIRS       := $(sort $(dir $(filter %_test.go,$(ALL_SRC))))
INTEG_TEST_DIRS := $(filter $(INTEG_TEST_ROOT)/ $(INTEG_TEST_NDC_ROOT)/,$(TEST_DIRS))
UNIT_TEST_DIRS  := $(filter-out $(INTEG_TEST_ROOT)% $(INTEG_TEST_XDC_ROOT)% $(INTEG_TEST_NDC_ROOT)%,$(TEST_DIRS))

# Code coverage output files.
COVER_ROOT                 := $(BUILD)/coverage
UNIT_COVER_FILE            := $(COVER_ROOT)/unit_cover.out
INTEG_COVER_FILE           := $(COVER_ROOT)/integ_$(PERSISTENCE_TYPE)_cover.out
INTEG_XDC_COVER_FILE       := $(COVER_ROOT)/integ_xdc_$(PERSISTENCE_TYPE)_cover.out
INTEG_CASS_COVER_FILE      := $(COVER_ROOT)/integ_cassandra_cover.out
INTEG_XDC_CASS_COVER_FILE  := $(COVER_ROOT)/integ_xdc_cassandra_cover.out
INTEG_SQL_COVER_FILE       := $(COVER_ROOT)/integ_sql_cover.out
INTEG_XDC_SQL_COVER_FILE   := $(COVER_ROOT)/integ_xdc_sql_cover.out
INTEG_NDC_COVER_FILE       := $(COVER_ROOT)/integ_ndc_$(PERSISTENCE_TYPE)_cover.out
INTEG_NDC_CASS_COVER_FILE  := $(COVER_ROOT)/integ_ndc_cassandra_cover.out
INTEG_NDC_SQL_COVER_FILE   := $(COVER_ROOT)/integ_ndc_sql_cover.out

# Need the following option to have integration tests count towards coverage. godoc below:
# -coverpkg pkg1,pkg2,pkg3
#   Apply coverage analysis in each test to the given list of packages.
#   The default is for each test to analyze only the package being tested.
#   Packages are specified as import paths.
GOCOVERPKG_ARG := -coverpkg="$(MODULE_ROOT)/common/...,$(MODULE_ROOT)/service/...,$(MODULE_ROOT)/client/...,$(MODULE_ROOT)/tools/..."

PROTO_ROOT     := proto
# Note: using "shell find" instead of "wildcard" because "wildcard" caches directory structure.
PROTO_DIRS     = $(sort $(dir $(shell find $(PROTO_ROOT) -name "*.proto" | grep -v temporal-proto)))
PROTO_SERVICES = $(shell find $(PROTO_ROOT) -name "*service.proto" | grep -v temporal-proto)
PROTO_IMPORT   := $(PROTO_ROOT):$(PROTO_ROOT)/temporal-proto:$(GOPATH)/src/github.com/gogo/protobuf/protobuf
PROTO_GEN      := .gen/proto

##### Tools #####
update-checkers:
	@printf $(COLOR) "Install/update check tools..."
	GO111MODULE=off go get -u golang.org/x/tools/cmd/goimports
	GO111MODULE=off go get -u golang.org/x/lint/golint
	GO111MODULE=off go get -u honnef.co/go/tools/cmd/staticcheck
	GO111MODULE=off go get -u github.com/kisielk/errcheck

update-mockgen:
	@printf $(COLOR) "Install/update mockgen tool..."
	GO111MODULE=off go get -u github.com/golang/mock/mockgen

update-proto-plugins:
	@printf $(COLOR) "Install/update proto plugins..."
	GO111MODULE=off go get -u github.com/gogo/protobuf/protoc-gen-gogoslick
	GO111MODULE=off go get -u google.golang.org/grpc

update-tools: update-checkers update-mockgen update-proto-plugins

##### Proto #####
$(PROTO_GEN):
	@mkdir -p $(PROTO_GEN)

clean-proto:
	@rm -rf $(PROTO_GEN)/*

update-proto-submodule:
	@printf $(COLOR) "Update proto submodule from remote..."
	git submodule update --remote $(PROTO_ROOT)/temporal-proto

install-proto-submodule:
	@printf $(COLOR) "Install proto submodule..."
	git submodule update --init $(PROTO_ROOT)/temporal-proto

protoc: $(PROTO_GEN)
	@printf $(COLOR) "Build proto files..."
# Run protoc separately for each directory because of different package names.
	$(foreach PROTO_DIR,$(PROTO_DIRS),protoc --proto_path=$(PROTO_IMPORT) --gogoslick_out=Mgoogle/protobuf/wrappers.proto=github.com/gogo/protobuf/types,Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types,plugins=grpc,paths=source_relative:$(PROTO_GEN) $(PROTO_DIR)*.proto$(NEWLINE))

# All gRPC generated service files pathes relative to PROTO_ROOT.
PROTO_GRPC_SERVICES = $(patsubst $(PROTO_GEN)/%,%,$(shell find $(PROTO_GEN) -name "service.pb.go"))
dir_no_slash = $(patsubst %/,%,$(dir $(1)))
dirname = $(notdir $(call dir_no_slash,$(1)))

proto-mock: $(PROTO_GEN)
	@printf $(COLOR) "Generate proto mocks..."
	$(foreach PROTO_GRPC_SERVICE,$(PROTO_GRPC_SERVICES),cd $(PROTO_GEN) && mockgen -package $(call dirname,$(PROTO_GRPC_SERVICE))mock -source $(PROTO_GRPC_SERVICE) -destination $(call dir_no_slash,$(PROTO_GRPC_SERVICE))mock/$(notdir $(PROTO_GRPC_SERVICE:go=mock.go))$(NEWLINE) )

update-proto-go:
	@printf $(COLOR) "Update go.temporal.io/temporal-proto..."
	@go get -u go.temporal.io/temporal-proto

proto: clean-proto install-proto-submodule protoc proto-mock

##### Binaries #####
clean-bins:
	@printf $(COLOR) "Delete old binaries..."
	@rm -f tctl
	@rm -f temporal-server
	@rm -f temporal-cassandra-tool
	@rm -f temporal-sql-tool
	@rm -f temporal-canary

temporal-server: proto
	@printf $(COLOR) "Build temporal-server with OS: $(GOOS), ARCH: $(GOARCH)..."
	go build -ldflags '$(GO_BUILD_LDFLAGS)' -o temporal-server cmd/server/main.go

tctl: proto
	@printf $(COLOR) "Build tctl with OS: $(GOOS), ARCH: $(GOARCH)..."
	go build -o tctl cmd/tools/cli/main.go

temporal-cassandra-tool: proto
	@printf $(COLOR) "Build temporal-cassandra-tool with OS: $(GOOS), ARCH: $(GOARCH)..."
	go build -o temporal-cassandra-tool cmd/tools/cassandra/main.go

temporal-sql-tool: proto
	@printf $(COLOR) "Build temporal-sql-tool with OS: $(GOOS), ARCH: $(GOARCH)..."
	go build -o temporal-sql-tool cmd/tools/sql/main.go

temporal-canary: proto
	@printf $(COLOR) "Build temporal-canary with OS: $(GOOS), ARCH: $(GOARCH)..."
	go build -o temporal-canary cmd/canary/main.go

##### Checks #####
copyright:
	@printf $(COLOR) "Check license header..."
	GOOS= GOARCH= go run ./cmd/tools/copyright/licensegen.go --verifyOnly

lint:
	@printf $(COLOR) "Run linter..."
	golint ./...

goimports-check:
	@printf $(COLOR) "Run goimports checks..."
# Use $(ALL_SRC) here to avoid checking generated files.
	@goimports -l $(ALL_SRC)

goimports:
	@printf $(COLOR) "Run goimports..."
	goimports -w $(ALL_SRC)

staticcheck:
	@printf $(COLOR) "Run staticcheck..."
	staticcheck -fail none ./...

errcheck:
	@printf $(COLOR) "Run errcheck..."
	errcheck ./... || true

check: copyright goimports-check lint staticcheck errcheck

##### Tests #####
clean-test-results:
	@rm -f test.log

unit-test: clean-test-results proto
	@printf $(COLOR) "Run unit tests..."
	$(foreach UNIT_TEST_DIR,$(UNIT_TEST_DIRS), @go test -timeout $(TEST_TIMEOUT) -race $(UNIT_TEST_DIR) $(TEST_TAG) | tee -a test.log$(NEWLINE))

integration-test: clean-test-results proto
	@printf $(COLOR) "Run integration tests..."
	$(foreach INTEG_TEST_DIR,$(INTEG_TEST_DIRS), @go test -timeout $(TEST_TIMEOUT) -race $(INTEG_TEST_DIR) $(TEST_TAG) | tee -a test.log$(NEWLINE))
# Need to run xdc tests with race detector off because of ringpop bug causing data race issue.
	@go test -timeout $(TEST_TIMEOUT) $(INTEG_TEST_XDC_ROOT) $(TEST_TAG) | tee -a test.log

test: unit-test integration-test

##### Coverage #####
clean-build-results:
	@rm -rf $(BUILD)
	@mkdir -p $(BUILD)
	@mkdir -p $(COVER_ROOT)

cover_profile: clean-build-results update-proto-plugins update-mockgen proto
	@echo "mode: atomic" > $(UNIT_COVER_FILE)

	@echo Running package tests:
	@for dir in $(UNIT_TEST_DIRS); do \
		mkdir -p $(BUILD)/"$$dir"; \
		go test "$$dir" $(TEST_ARG) -coverprofile=$(BUILD)/"$$dir"/coverage.out || exit 1; \
		cat $(BUILD)/"$$dir"/coverage.out | grep -v "^mode: \w\+" >> $(UNIT_COVER_FILE); \
	done;

cover_integration_profile: clean-build-results update-proto-plugins update-mockgen proto
	@echo "mode: atomic" > $(INTEG_COVER_FILE)

	@echo Running integration test with $(PERSISTENCE_TYPE)
	@mkdir -p $(BUILD)/$(INTEG_TEST_OUT_DIR)
	@time go test $(INTEG_TEST_ROOT) $(TEST_ARG) $(TEST_TAG) -persistenceType=$(PERSISTENCE_TYPE) $(GOCOVERPKG_ARG) -coverprofile=$(BUILD)/$(INTEG_TEST_OUT_DIR)/coverage.out || exit 1;
	@cat $(BUILD)/$(INTEG_TEST_OUT_DIR)/coverage.out | grep -v "^mode: \w\+" >> $(INTEG_COVER_FILE)

cover_xdc_profile: clean-build-results update-proto-plugins update-mockgen proto
	@echo "mode: atomic" > $(INTEG_XDC_COVER_FILE)

	@echo Running integration test for cross dc with $(PERSISTENCE_TYPE)
	@mkdir -p $(BUILD)/$(INTEG_TEST_XDC_OUT_DIR)
	@time go test -v -timeout $(TEST_TIMEOUT) $(INTEG_TEST_XDC_ROOT) $(TEST_TAG) -persistenceType=$(PERSISTENCE_TYPE) $(GOCOVERPKG_ARG) -coverprofile=$(BUILD)/$(INTEG_TEST_XDC_OUT_DIR)/coverage.out || exit 1;
	@cat $(BUILD)/$(INTEG_TEST_XDC_OUT_DIR)/coverage.out | grep -v "^mode: \w\+" | grep -v "mode: set" >> $(INTEG_XDC_COVER_FILE)

cover_ndc_profile: clean-build-results update-proto-plugins update-mockgen proto
	@mkdir -p $(BUILD)
	@mkdir -p $(COVER_ROOT)
	@echo "mode: atomic" > $(INTEG_NDC_COVER_FILE)

	@echo Running integration test for 3+ dc with $(PERSISTENCE_TYPE)
	@mkdir -p $(BUILD)/$(INTEG_TEST_NDC_OUT_DIR)
	@time go test -v -timeout $(TEST_TIMEOUT) $(INTEG_TEST_NDC_ROOT) $(TEST_TAG) -persistenceType=$(PERSISTENCE_TYPE) $(GOCOVERPKG_ARG) -coverprofile=$(BUILD)/$(INTEG_TEST_NDC_OUT_DIR)/coverage.out -count=$(TEST_RUN_COUNT) || exit 1;
	@cat $(BUILD)/$(INTEG_TEST_NDC_OUT_DIR)/coverage.out | grep -v "^mode: \w\+" | grep -v "mode: set" >> $(INTEG_NDC_COVER_FILE)

$(COVER_ROOT)/cover.out: $(UNIT_COVER_FILE) $(INTEG_CASS_COVER_FILE) $(INTEG_XDC_CASS_COVER_FILE) $(INTEG_SQL_COVER_FILE) $(INTEG_XDC_SQL_COVER_FILE)
	@echo "mode: atomic" > $(COVER_ROOT)/cover.out
	cat $(UNIT_COVER_FILE) | grep -v "^mode: \w\+" | grep -vP ".gen|[Mm]ock[s]?" >> $(COVER_ROOT)/cover.out
	cat $(INTEG_CASS_COVER_FILE) | grep -v "^mode: \w\+" | grep -vP ".gen|[Mm]ock[s]?" >> $(COVER_ROOT)/cover.out
	cat $(INTEG_XDC_CASS_COVER_FILE) | grep -v "^mode: \w\+" | grep -vP ".gen|[Mm]ock[s]?" >> $(COVER_ROOT)/cover.out
	cat $(INTEG_SQL_COVER_FILE) | grep -v "^mode: \w\+" | grep -vP ".gen|[Mm]ock[s]?" >> $(COVER_ROOT)/cover.out
	cat $(INTEG_XDC_SQL_COVER_FILE) | grep -v "^mode: \w\+" | grep -vP ".gen|[Mm]ock[s]?" >> $(COVER_ROOT)/cover.out

cover: $(COVER_ROOT)/cover.out
	go tool cover -html=$(COVER_ROOT)/cover.out;

cover_ci: $(COVER_ROOT)/cover.out
	goveralls -coverprofile=$(COVER_ROOT)/cover.out -service=buildkite || echo Coveralls failed;

##### Schema #####
install-schema: temporal-cassandra-tool
	@printf $(COLOR) "Install Cassandra schema..."
	./temporal-cassandra-tool --ep 127.0.0.1 create -k temporal --rf 1
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal setup-schema -v 0.0
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal update-schema -d ./schema/cassandra/temporal/versioned
	./temporal-cassandra-tool --ep 127.0.0.1 create -k temporal_visibility --rf 1
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_visibility setup-schema -v 0.0
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_visibility update-schema -d ./schema/cassandra/visibility/versioned

install-schema-mysql-pre5720: temporal-sql-tool
	@printf $(COLOR) "Install MySQL schema..."
	./temporal-sql-tool --ep 127.0.0.1 --ca tx_isolation='READ-COMMITTED' create --db temporal
	./temporal-sql-tool --ep 127.0.0.1 --ca tx_isolation='READ-COMMITTED' --db temporal setup-schema -v 0.0
	./temporal-sql-tool --ep 127.0.0.1 --ca tx_isolation='READ-COMMITTED' --db temporal update-schema -d ./schema/mysql/v57/temporal/versioned
	./temporal-sql-tool --ep 127.0.0.1 --ca tx_isolation='READ-COMMITTED' create --db temporal_visibility
	./temporal-sql-tool --ep 127.0.0.1 --ca tx_isolation='READ-COMMITTED' --db temporal_visibility setup-schema -v 0.0
	./temporal-sql-tool --ep 127.0.0.1 --ca tx_isolation='READ-COMMITTED' --db temporal_visibility update-schema -d ./schema/mysql/v57/visibility/versioned

install-schema-mysql: temporal-sql-tool
	@printf $(COLOR) "Install MySQL schema..."
	./temporal-sql-tool --ep 127.0.0.1 create --db temporal
	./temporal-sql-tool --ep 127.0.0.1 --db temporal setup-schema -v 0.0
	./temporal-sql-tool --ep 127.0.0.1 --db temporal update-schema -d ./schema/mysql/v57/temporal/versioned
	./temporal-sql-tool --ep 127.0.0.1 create --db temporal_visibility
	./temporal-sql-tool --ep 127.0.0.1 --db temporal_visibility setup-schema -v 0.0
	./temporal-sql-tool --ep 127.0.0.1 --db temporal_visibility update-schema -d ./schema/mysql/v57/visibility/versioned

install-schema-postgres: temporal-sql-tool
	@printf $(COLOR) "Install Postgres schema..."
	./temporal-sql-tool --ep 127.0.0.1 -p 5432 -u postgres -pw temporal --pl postgres create --db temporal
	./temporal-sql-tool --ep 127.0.0.1 -p 5432 -u postgres -pw temporal --pl postgres --db temporal setup -v 0.0
	./temporal-sql-tool --ep 127.0.0.1 -p 5432 -u postgres -pw temporal --pl postgres --db temporal update-schema -d ./schema/postgres/temporal/versioned
	./temporal-sql-tool --ep 127.0.0.1 -p 5432 -u postgres -pw temporal --pl postgres create --db temporal_visibility
	./temporal-sql-tool --ep 127.0.0.1 -p 5432 -u postgres -pw temporal --pl postgres --db temporal_visibility setup-schema -v 0.0
	./temporal-sql-tool --ep 127.0.0.1 -p 5432 -u postgres -pw temporal --pl postgres --db temporal_visibility update-schema -d ./schema/postgres/visibility/versioned

install-schema-cdc: temporal-cassandra-tool
	@printf $(COLOR)  "Set up temporal_active key space..."
	./temporal-cassandra-tool --ep 127.0.0.1 create -k temporal_active --rf 1
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_active setup-schema -v 0.0
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_active update-schema -d ./schema/cassandra/temporal/versioned
	./temporal-cassandra-tool --ep 127.0.0.1 create -k temporal_visibility_active --rf 1
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_visibility_active setup-schema -v 0.0
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_visibility_active update-schema -d ./schema/cassandra/visibility/versioned

	@printf $(COLOR) "Set up temporal_standby key space..."
	./temporal-cassandra-tool --ep 127.0.0.1 create -k temporal_standby --rf 1
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_standby setup-schema -v 0.0
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_standby update-schema -d ./schema/cassandra/temporal/versioned
	./temporal-cassandra-tool --ep 127.0.0.1 create -k temporal_visibility_standby --rf 1
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_visibility_standby setup-schema -v 0.0
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_visibility_standby update-schema -d ./schema/cassandra/visibility/versioned

	@printf $(COLOR) "Set up temporal_other key space..."
	./temporal-cassandra-tool --ep 127.0.0.1 create -k temporal_other --rf 1
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_other setup-schema -v 0.0
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_other update-schema -d ./schema/cassandra/temporal/versioned
	./temporal-cassandra-tool --ep 127.0.0.1 create -k temporal_visibility_other --rf 1
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_visibility_other setup-schema -v 0.0
	./temporal-cassandra-tool --ep 127.0.0.1 -k temporal_visibility_other update-schema -d ./schema/cassandra/visibility/versioned

##### Start #####
start: temporal-server
	./temporal-server start

start-cdc-active: temporal-server
	./temporal-server --zone active start

start-cdc-standby: temporal-server
	./temporal-server --zone standby start

start-cdc-other: temporal-server
	./temporal-server --zone other start

start-canary: temporal-canary
	./temporal-canary start

##### Auxilary #####
go-generate:
	@printf $(COLOR) "Regenerate everything..."
	@go generate ./...

gomodtidy:
	@printf $(COLOR) "go mod tidy..."
	@go mod tidy