package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"payment-kita.backend/internal/config"
	"payment-kita.backend/internal/infrastructure/models"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := config.Load()

	if cfg.Database.DBLegacy == "" || cfg.Database.DBApp == "" {
		log.Fatal("DB_LEGACY and DB_APP must be set in environment")
	}

	fmt.Printf("🚀 Starting migration from %s to %s\n", cfg.Database.DBLegacy, cfg.Database.DBApp)

	// Connect to Legacy DB (Source)
	dbLegacy, err := gorm.Open(postgres.Open(cfg.Database.DBLegacy), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to Legacy DB: %v", err)
	}

	// Connect to App DB (Destination)
	dbApp, err := gorm.Open(postgres.Open(cfg.Database.DBApp), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		log.Fatalf("Failed to connect to App DB: %v", err)
	}

	log.Println("🧹 Cleaning App DB (Dropping existing tables and types)...")
	// Drop all tables in public schema
	var tables []string
	dbApp.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'").Scan(&tables)
	for _, table := range tables {
		log.Printf("   Dropping table %s...", table)
		dbApp.Exec(fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE", table))
	}
	// Drop all enums
	var types []string
	dbApp.Raw("SELECT typname FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = 'public' AND t.typtype = 'e'").Scan(&types)
	for _, t := range types {
		log.Printf("   Dropping type %s...", t)
		dbApp.Exec(fmt.Sprintf("DROP TYPE IF EXISTS \"%s\" CASCADE", t))
	}

	log.Println("🛠️ Preparing App DB (Extensions & Functions)...")
	dbApp.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")
	dbApp.Exec("CREATE EXTENSION IF NOT EXISTS \"pgcrypto\"")
	dbApp.Exec(`
		CREATE OR REPLACE FUNCTION uuid_generate_v7()
		RETURNS uuid AS $$
		DECLARE
		  unix_ts_ms bytea;
		  uuid_bytes bytea;
		BEGIN
		  unix_ts_ms = substring(int8send(floor(extract(epoch from clock_timestamp()) * 1000)::bigint) from 3);
		  uuid_bytes = unix_ts_ms || gen_random_bytes(10);
		  uuid_bytes = set_byte(uuid_bytes, 6, (get_byte(uuid_bytes, 6) & x'0f'::int) | x'70'::int);
		  uuid_bytes = set_byte(uuid_bytes, 8, (get_byte(uuid_bytes, 8) & x'3f'::int) | x'80'::int);
		  RETURN encode(uuid_bytes, 'hex')::uuid;
		END;
		$$ LANGUAGE plpgsql;
	`)

	log.Println("🛠️ Preparing App DB (Enums)...")
	enums := map[string][]string{
		"user_role_enum":              {"ADMIN", "SUB_ADMIN", "PARTNER", "USER"},
		"merchant_type_enum":          {"PARTNER", "CORPORATE", "UMKM", "RETAIL"},
		"merchant_status_enum":        {"PENDING", "ACTIVE", "SUSPENDED", "REJECTED"},
		"kyc_status_enum":             {"NOT_STARTED", "ID_CARD_VERIFIED", "FACE_VERIFIED", "LIVENESS_VERIFIED", "FULLY_VERIFIED"},
		"chain_type_enum":             {"EVM", "SVM", "MoveVM", "PolkaVM", "COSMOS"},
		"token_type_enum":             {"NATIVE", "ERC20", "SPL", "COIN"},
		"payment_status_enum":         {"PENDING", "PROCESSING", "COMPLETED", "FAILED", "REFUNDED"},
		"payment_request_status_enum": {"PENDING", "COMPLETED", "EXPIRED", "CANCELLED"},
		"job_status_enum":             {"PENDING", "PROCESSING", "COMPLETED", "FAILED"},
		"webhook_delivery_status":    {"pending", "delivering", "delivered", "retrying", "failed", "dropped"},
	}

	for name, values := range enums {
		var exists bool
		dbApp.Raw("SELECT EXISTS (SELECT 1 FROM pg_type WHERE typname = ?)", name).Scan(&exists)
		if !exists {
			valList := ""
			for i, v := range values {
				if i > 0 {
					valList += ", "
				}
				valList += fmt.Sprintf("'%s'", v)
			}
			dbApp.Exec(fmt.Sprintf("CREATE TYPE %s AS ENUM (%s)", name, valList))
		}
	}

	log.Println("🛠️ Running AutoMigrate on App DB...")
	allModels := []interface{}{
		&models.Chain{},
		&models.Token{},
		&models.User{},
		&models.Merchant{},
		&models.ChainRPC{},
		&models.EmailVerification{},
		&models.MerchantSettlementProfile{},
		&models.SmartContract{},
		&models.Wallet{},
		&models.PaymentBridge{},
		&models.BridgeConfig{},
		&models.FeeConfig{},
		&models.PaymentRequest{},
		&models.Payment{},
		&models.PaymentEvent{},
		&models.WebhookLog{},
		&models.BackgroundJob{},
		&models.ApiKey{},
		&models.Team{},
		&models.RoutePolicy{},
		&models.StargateConfig{},
	}

	for _, m := range allModels {
		if err := dbApp.AutoMigrate(m); err != nil {
			log.Fatalf("❌ Failed migrating schema for %T: %v", m, err)
		}
	}

	ctx := context.Background()

	// Migration Order (respecting foreign keys)
	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.Chain{})
	if err != nil { fmt.Printf("⚠️ Warning migrating Chain: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.ChainRPC{})
	if err != nil { fmt.Printf("⚠️ Warning migrating ChainRPC: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.Token{})
	if err != nil { fmt.Printf("⚠️ Warning migrating Token: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.User{})
	if err != nil { fmt.Printf("⚠️ Warning migrating User: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.EmailVerification{})
	if err != nil { fmt.Printf("⚠️ Warning migrating EmailVerification: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.Merchant{})
	if err != nil { fmt.Printf("⚠️ Warning migrating Merchant: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.MerchantSettlementProfile{})
	if err != nil { fmt.Printf("⚠️ Warning migrating MerchantSettlementProfile: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.SmartContract{})
	if err != nil { fmt.Printf("⚠️ Warning migrating SmartContract: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.Wallet{})
	if err != nil { fmt.Printf("⚠️ Warning migrating Wallet: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.PaymentBridge{})
	if err != nil { fmt.Printf("⚠️ Warning migrating PaymentBridge: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.BridgeConfig{})
	if err != nil { fmt.Printf("⚠️ Warning migrating BridgeConfig: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.FeeConfig{})
	if err != nil { fmt.Printf("⚠️ Warning migrating FeeConfig: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.PaymentRequest{})
	if err != nil { fmt.Printf("⚠️ Warning migrating PaymentRequest: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.Payment{})
	if err != nil { fmt.Printf("⚠️ Warning migrating Payment: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.PaymentEvent{})
	if err != nil { fmt.Printf("⚠️ Warning migrating PaymentEvent: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.WebhookLog{})
	if err != nil { fmt.Printf("⚠️ Warning migrating WebhookLog: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.BackgroundJob{})
	if err != nil { fmt.Printf("⚠️ Warning migrating BackgroundJob: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.ApiKey{})
	if err != nil { fmt.Printf("⚠️ Warning migrating ApiKey: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.Team{})
	if err != nil { fmt.Printf("⚠️ Warning migrating Team: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.RoutePolicy{})
	if err != nil { fmt.Printf("⚠️ Warning migrating RoutePolicy: %v\n", err) }

	err = migrateTable(ctx, dbLegacy, dbApp, &[]models.StargateConfig{})
	if err != nil { fmt.Printf("⚠️ Warning migrating StargateConfig: %v\n", err) }

	fmt.Println("✅ Migration completed!")
}

func migrateTable(ctx context.Context, src *gorm.DB, dest *gorm.DB, modelSlice interface{}) error {
	typeName := fmt.Sprintf("%T", modelSlice)
	fmt.Printf("📦 Migrating %s...\n", typeName)

	// Fetch all from source
	res := src.Find(modelSlice)
	if res.Error != nil {
		return fmt.Errorf("failed to fetch from source: %w", res.Error)
	}

	if res.RowsAffected == 0 {
		fmt.Printf("ℹ️ Skipping %s: no records found in source\n", typeName)
		return nil
	}

	// Insert into destination with conflict handling
	if err := dest.Clauses(clause.OnConflict{DoNothing: true}).Create(modelSlice).Error; err != nil {
		return fmt.Errorf("failed to insert into destination: %w", err)
	}

	return nil
}
