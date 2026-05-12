.PHONY: verify-broken verify-fixed fix test

# Verify the project is in a broken state (test fails)
verify-broken:
	@echo "=== Verifying broken-project (expecting FAIL) ==="
	@cd broken-project && go test ./... 2>&1; \
	if [ $$? -eq 0 ]; then \
		echo "ERROR: tests passed but should have failed!"; \
		exit 1; \
	fi; \
	echo "=== OK: tests fail as expected ==="

# Verify the project passes after fix
verify-fixed:
	@echo "=== Verifying fixed state (expecting PASS) ==="
	@cd broken-project && go test ./... 2>&1; \
	if [ $$? -ne 0 ]; then \
		echo "ERROR: tests failed but should have passed!"; \
		exit 1; \
	fi; \
	echo "=== OK: all tests pass ==="

# Apply the fix to broken-project
fix:
	@cd broken-project && sed -i.bak 's|return total / float64(len(nums))|if len(nums) == 0 {\n\t\treturn 0\n\t}\n\treturn total / float64(len(nums))|' calculator.go && rm -f calculator.go.bak

# Full verification: show broken state, apply fix, show fixed state
test:
	@echo "=== ByteMind Bugfix Demo ==="
	@echo ""
	@echo "Step 1: Check initial state (should be broken)..."
	@cd broken-project && go test ./... 2>&1 || true
	@echo ""
	@echo "Step 2: Apply fix..."
	$(MAKE) fix
	@echo ""
	@echo "Step 3: Verify fix..."
	$(MAKE) verify-fixed
	@echo ""
	@echo "=== Demo verification complete ==="
