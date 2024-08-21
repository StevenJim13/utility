PROGRAM=utility
REMOVE=@rm -f
PRINT=@echo
GO=@go

COVERFILE=cover.out
COVERHTML=coverage.html

test:
	$(GO) test -v ./... -coverprofile=$(COVERFILE)
	$(PRINT) "Start render coverage report to $(COVERHTML)."
	$(GO) tool cover --html=cover.out -o $(COVERHTML)
	$(PRINT) "create coverage report at: $(COVERHTML)."

mock:
	$(GO) generate ./...
	$(PRINT) "Successfully generate mock files."

clean:
	$(REMOVE) $(COVERFILE) $(COVERHTML) main.go
	$(PRINT) "Clean up done"

env:
	$(PRINT) "Initializing environment..."
	$(GO) mod download
	$(PRINT) "Successfully downloaded dependencies."
	@./scripts/init_env.sh
	$(PRINT) "Successfully installed build toolchain."

.PHONY: test mock clean env
