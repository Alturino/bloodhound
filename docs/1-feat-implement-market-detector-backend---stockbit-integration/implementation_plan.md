# Implement Market Detector Backend - Stockbit Integration (Updated)

This implementation plan details the steps required to align the backend service with your manual refactors to the domain models and migrations.

## User Review Required

> [!IMPORTANT]
> **Database Migrations & Upsert Strategy**: Following your feedback, `broker_transactions` will now have a formal unique constraint on `(symbol, trade_date, broker_code, side)`. The upsert logic will shift from delete-and-insert to `ON CONFLICT DO UPDATE`. 

> [!NOTE]
> **Precision**: Switched to `shopspring/decimal` for all financial data to ensure high-precision handling of Stockbit's scientific notation strings.

> [!CAUTION]
> The database name used for Jet model generation may be `postgres` or `bloodhound` based on past bash histories. The plan assumes `bloodhound` as defined in `config.yaml`. Let me know if another DB needs to be targeted during the Jet code generation step.

## Proposed Changes

### Memory Optimization & Structural Alignment
Optimize all structs for memory alignment by reordering fields from the smallest byte size to the largest.

1. **Byte Size References (64-bit system)**:
   - `int8`, `bool`: 1 byte
   - `int16`: 2 bytes
   - `int32`: 4 bytes
   - `int`, `int64`, `float64`: 8 bytes
   - `string`, `decimal.Decimal`: 16 bytes
   - `slice`, `interface`: 24 bytes

2. **[MODIFY] internal/models/stockbit.go**:
   Reorder fields in all models (Response, Data, BandarDetector, BrokerSummary, etc.) following the "smallest to largest" rule.

---

### Database Schema & Model Generation Alignment
Ensure the database schema enforces `NOT NULL` across all market detector fields to eliminate pointers in the generated models.

1. **[MODIFY] migrations/20260408105000_create_market_detectors.up.sql**:
   Verify all columns (except sequence/defaults) are marked `NOT NULL`.
2. **Execute Migration Refresh**:
   - `migrate -path migrations -database "${DSN}" down 1`
   - `migrate -path migrations -database "${DSN}" up 1`
3. **Regenerate Type-Safe Models**:
   - Run `jet` to generate models without pointers for `NOT NULL` fields.

---

### Logic Alignment & Build Fixes
Correct the fallout in the integration layers.

1. **[MODIFY] internal/stockbit/client.go**:
   - Remove null checks for `Data` (switch to empty-struct checks if necessary).
   - Ensure response parsing handles the reordered fields.
2. **[MODIFY] internal/state/db_store.go**:
   - Update mapping logic to use value assignment (not pointers).
   - Resolve any reordered field mismatches during initialization.
3. **[MODIFY] internal/models/stockbit_test.go**:
   - Update test expectations and struct initializers.

## Open Questions

> [!IMPORTANT]
> 1. **Authentication:** Should the `STOCKBIT_TOKEN` be sourced via environment variable, or parsed out of a login endpoint if it expires? For now, I will add it to the `config.yaml` / Viper configuration.
> 2. **DB Authentication:** I will need the correct credentials for the local postgres instance to test queries and execute `go-jet`. Based on the `docker-compose.yml`, `postgres/postgres` is typical, but please confirm its availability during execution.

## Verification Plan

### Automated Verification
- Provide mock tests using a mock response matching the JSON payload above in `internal/stockbit/client_test.go` to assert the scientific notation conversions execute correctly.
- Test `go-jet` schema generation against the applied database migrations.

### Manual Verification
- A background test or manual invocation script against your PostgreSQL instance to confirm `ON CONFLICT` functionality and ensuring rows duplicate constraints apply seamlessly.
- Log inspecting `slog` outputs validating API hit success to https://exodus.stockbit.com/marketdetectors/{symbol}.
