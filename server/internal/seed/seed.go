package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cool-dispatch/internal/config"
	"cool-dispatch/internal/models"
	"cool-dispatch/internal/security"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SyncAdminFromConfig 无条件将 config.yaml / 环境变量中的管理员配置同步到数据库。
// 只要配置中提供了管理员名称、手机号和密码，每次启动都会 upsert 系统中唯一的 admin 账号，
// 确保管理员凭证变更后无需手动操作数据库即可生效，同时吊销旧令牌强制重新登录。
// 若配置中未提供管理员信息（三项任一为空），则静默跳过，不影响正常启动。
func SyncAdminFromConfig(db *gorm.DB, cfg config.Config) error {
	adminName := strings.TrimSpace(cfg.SeedAdminName)
	adminPhone := strings.TrimSpace(cfg.SeedAdminPhone)
	adminPassword := strings.TrimSpace(cfg.SeedAdminPassword)

	// 三项配置任一为空时静默跳过，不阻塞启动
	if adminName == "" || adminPhone == "" || adminPassword == "" {
		return nil
	}

	// 密码长度不够时报错，防止设置了弱密码
	if len(adminPassword) < security.PasswordMinLength {
		return fmt.Errorf("config admin password must be at least %d characters", security.PasswordMinLength)
	}

	return upsertConfiguredAdminUser(db, adminName, adminPhone, adminPassword)
}

// SyncDevTechnicianFromConfig 无条件将 config.yaml / 环境变量中的开发默认师傅配置同步到数据库。
// 仅当三项配置（名称、手机号、密码）都提供时才执行，保证开发人员始终有一个可直接登录的师傅端账号。
// 每次启动时 upsert：若该手机号对应的 technician 已存在则更新密码，否则新建。
func SyncDevTechnicianFromConfig(db *gorm.DB, cfg config.Config) error {
	techName := strings.TrimSpace(cfg.DevTechnicianName)
	techPhone := strings.TrimSpace(cfg.DevTechnicianPhone)
	techPassword := strings.TrimSpace(cfg.DevTechnicianPassword)

	// 三项配置任一为空时静默跳过，不阻塞启动
	if techName == "" || techPhone == "" || techPassword == "" {
		return nil
	}

	// 密码长度不够时报错，防止设置了弱密码
	if len(techPassword) < security.PasswordMinLength {
		return fmt.Errorf("config dev technician password must be at least %d characters", security.PasswordMinLength)
	}

	techPasswordHash, err := security.HashPassword(techPassword)
	if err != nil {
		return fmt.Errorf("hash config dev technician password: %w", err)
	}

	// 按手机号查找已有 technician；存在则更新，不存在则创建
	var existing models.User
	err = db.First(&existing, "phone = ? AND role = ?", techPhone, "technician").Error
	if err == nil {
		// 已存在：更新名称和密码
		updates := map[string]any{
			"name":          techName,
			"password_hash": techPasswordHash,
		}
		if err := db.Model(&existing).Updates(updates).Error; err != nil {
			return fmt.Errorf("update dev technician: %w", err)
		}
		return nil
	}
	if !isNotFound(err) {
		return fmt.Errorf("query dev technician: %w", err)
	}

	// 不存在：创建新的开发师傅账号
	userID, err := nextSeedUserID(db)
	if err != nil {
		return err
	}
	defaultColor := "#1677ff"
	newTech := models.User{
		ID:           userID,
		Name:         techName,
		Role:         "technician",
		Phone:        techPhone,
		PasswordHash: techPasswordHash,
		Color:        &defaultColor,
		Skills:       jsonArray([]string{}),
		Availability: jsonArray([]string{}),
	}
	if err := db.Create(&newTech).Error; err != nil {
		return fmt.Errorf("create dev technician: %w", err)
	}
	return nil
}

// SeedDemoData 为本地开发和联调准备一套最小可用的演示数据，避免前端空白。
// Demo 账号密码与默认管理员账号必须通过配置显式提供，避免继续把默认口令和登录账号写死在代码里。
func SeedDemoData(db *gorm.DB, cfg config.Config) error {
	seeders := []func(*gorm.DB) error{
		func(tx *gorm.DB) error { return seedUsers(tx, cfg) },
		seedSettings,
		seedServiceItems,
		seedExtraItems,
		seedZones,
		seedCustomers,
		seedAppointments,
		seedCashLedgerEntries,
		seedReviews,
		seedNotificationLogs,
		seedLineFriends,
	}

	for _, seeder := range seeders {
		if err := seeder(db); err != nil {
			return err
		}
	}

	return nil
}

// seedUsers 初始化登录和派工依赖的账号数据。
// 为了避免仓库内继续存在默认管理员账号与口令，管理员登录信息由配置文件或环境变量显式提供。
// 只要启用了 demo seed，就会优先用显式配置覆盖数据库中的管理员账号与密码，
// 确保 config.yaml 始终是当前开发环境的管理员登录真相来源。
func seedUsers(db *gorm.DB, cfg config.Config) error {
	adminName := strings.TrimSpace(cfg.SeedAdminName)
	adminPhone := strings.TrimSpace(cfg.SeedAdminPhone)
	adminPassword := strings.TrimSpace(cfg.SeedAdminPassword)
	technicianPassword := strings.TrimSpace(cfg.SeedTechnicianPassword)
	if adminName == "" {
		return fmt.Errorf("seed admin name is required")
	}
	if adminPhone == "" {
		return fmt.Errorf("seed admin phone is required")
	}
	if len(adminPassword) < security.PasswordMinLength {
		return fmt.Errorf("seed admin password is required and must be at least %d characters", security.PasswordMinLength)
	}
	if len(technicianPassword) < security.PasswordMinLength {
		return fmt.Errorf("seed technician password is required and must be at least %d characters", security.PasswordMinLength)
	}

	technicianPasswordHash, err := security.HashPassword(technicianPassword)
	if err != nil {
		return fmt.Errorf("hash seed technician password: %w", err)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := upsertConfiguredAdminUser(tx, adminName, adminPhone, adminPassword); err != nil {
			return err
		}

		var technicianCount int64
		if err := tx.Model(&models.User{}).Where("role = ?", "technician").Count(&technicianCount).Error; err != nil {
			return fmt.Errorf("count technicians: %w", err)
		}
		if technicianCount > 0 {
			return nil
		}

		return createDemoTechnicians(tx, technicianPasswordHash)
	})
}

// upsertConfiguredAdminUser 确保数据库中的管理员账号与当前配置保持一致。
// 仅当手机号或密码发生真实变化时才吊销现有 token，避免服务重启/更新后无故退出登录。
func upsertConfiguredAdminUser(db *gorm.DB, adminName, adminPhone, adminPassword string) error {
	adminPasswordHash, err := security.HashPassword(adminPassword)
	if err != nil {
		return fmt.Errorf("hash config admin password: %w", err)
	}

	var admin models.User
	err = db.Where("role = ?", "admin").Order("id asc").First(&admin).Error
	if err == nil {
		if err := ensureSeedAdminPhoneAvailable(db, admin.ID, adminPhone); err != nil {
			return err
		}

		adminNameChanged := admin.Name != adminName
		adminPhoneChanged := admin.Phone != adminPhone
		adminPasswordChanged := !security.VerifyPassword(adminPassword, admin.PasswordHash)
		if !adminNameChanged && !adminPhoneChanged && !adminPasswordChanged {
			return nil
		}

		updates := map[string]any{
			"name":  adminName,
			"phone": adminPhone,
			"role":  "admin",
		}
		if adminPasswordChanged {
			updates["password_hash"] = adminPasswordHash
		}

		if err := db.Model(&admin).Updates(updates).Error; err != nil {
			return fmt.Errorf("update configured admin user: %w", err)
		}

		if adminPhoneChanged || adminPasswordChanged {
			if err := db.Where("user_id = ?", admin.ID).Delete(&models.AuthToken{}).Error; err != nil {
				return fmt.Errorf("revoke configured admin tokens: %w", err)
			}
		}
		return nil
	}
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("query admin user: %w", err)
	}

	if err := ensureSeedAdminPhoneAvailable(db, 0, adminPhone); err != nil {
		return err
	}

	userID, err := nextSeedUserID(db)
	if err != nil {
		return err
	}
	if userID == 0 {
		userID = 1
	}

	admin = models.User{
		ID:           userID,
		Name:         adminName,
		Role:         "admin",
		Phone:        adminPhone,
		PasswordHash: adminPasswordHash,
		Skills:       jsonArray([]string{}),
		Availability: jsonArray([]string{}),
	}
	if err := db.Create(&admin).Error; err != nil {
		return fmt.Errorf("create configured admin user: %w", err)
	}
	return nil
}

// ensureSeedAdminPhoneAvailable 校验目标管理员手机号不会与其它账号冲突，避免把配置误覆盖到非管理员账号。
func ensureSeedAdminPhoneAvailable(db *gorm.DB, currentUserID uint, adminPhone string) error {
	var existing models.User
	err := db.First(&existing, "phone = ?", adminPhone).Error
	if isNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("query configured admin phone owner: %w", err)
	}
	if currentUserID != 0 && existing.ID == currentUserID {
		return nil
	}
	return fmt.Errorf("seed admin phone already belongs to user %d", existing.ID)
}

// createDemoTechnicians 仅在库中还没有技师账号时写入默认 demo 技师，避免重复覆盖现有技师资料。
func createDemoTechnicians(db *gorm.DB, technicianPasswordHash string) error {
	zone1 := "zone-1"
	zone2 := "zone-2"
	indigo := "#4f46e5"
	emerald := "#059669"
	amber := "#d97706"
	baseID, err := nextSeedUserID(db)
	if err != nil {
		return err
	}

	users := []models.User{
		{
			ID: baseID, Name: "王師傅", Role: "technician", Phone: "0987654321", Color: &indigo, ZoneID: &zone1,
			PasswordHash: technicianPasswordHash,
			Skills:       jsonArray([]string{"分離式", "吊隱式"}),
			Availability: jsonRaw(`[{"day":1,"slots":["09:00","10:00","11:00","13:00","14:00","15:00","16:00"]},{"day":2,"slots":["09:00","10:00","11:00","13:00","14:00","15:00","16:00"]},{"day":3,"slots":["09:00","10:00","11:00","13:00","14:00","15:00","16:00"]}]`),
		},
		{
			ID: baseID + 1, Name: "李師傅", Role: "technician", Phone: "0911111111", Color: &emerald, ZoneID: &zone2,
			PasswordHash: technicianPasswordHash,
			Skills:       jsonArray([]string{"分離式", "窗型"}),
			Availability: jsonRaw(`[{"day":1,"slots":["08:00","09:00","10:00","11:00","12:00"]},{"day":3,"slots":["08:00","09:00","10:00","11:00","12:00"]},{"day":5,"slots":["08:00","09:00","10:00","11:00","12:00"]}]`),
		},
		{
			ID: baseID + 2, Name: "陳師傅", Role: "technician", Phone: "0922222222", Color: &amber, ZoneID: &zone1,
			PasswordHash: technicianPasswordHash,
			Skills:       jsonArray([]string{"分離式", "吊隱式", "窗型"}),
			Availability: jsonRaw(`[{"day":2,"slots":["13:00","14:00","15:00","16:00","17:00","18:00"]},{"day":4,"slots":["13:00","14:00","15:00","16:00","17:00","18:00"]},{"day":6,"slots":["09:00","10:00","11:00","12:00"]}]`),
		},
	}

	return db.Create(&users).Error
}

// nextSeedUserID 计算当前 users 表中下一个可用主键，避免新建管理员时与既有数据冲突。
func nextSeedUserID(db *gorm.DB) (uint, error) {
	var currentMax uint
	if err := db.Model(&models.User{}).Select("COALESCE(MAX(id), 0)").Scan(&currentMax).Error; err != nil {
		return 0, fmt.Errorf("query next seed user id: %w", err)
	}
	return currentMax + 1, nil
}

// isNotFound 统一兼容 gorm 的“记录不存在”判断，避免多处分支重复导入与比较。
func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// seedSettings 初始化系统级配置，当前先补齐回访提醒天数。
func seedSettings(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.AppSetting{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count settings: %w", err)
	}
	if count > 0 {
		return nil
	}

	settings := []models.AppSetting{
		{Key: "reminder_days", Value: "180"},
	}
	return db.Create(&settings).Error
}

// seedServiceItems 初始化预约创建页使用的服务项目。
func seedServiceItems(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.ServiceItem{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count service items: %w", err)
	}
	if count > 0 {
		return nil
	}

	items := []models.ServiceItem{
		{ID: "si-1", Name: "分離式", DefaultPrice: 2500, Description: strPtr("壁掛式室內機 + 室外機")},
		{ID: "si-2", Name: "吊隱式", DefaultPrice: 3500, Description: strPtr("隱藏於天花板內的機型")},
		{ID: "si-3", Name: "窗型", DefaultPrice: 2000, Description: strPtr("窗戶安裝一體機")},
	}

	return db.Create(&items).Error
}

// seedExtraItems 初始化額外費用選項。
func seedExtraItems(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.ExtraItem{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count extra items: %w", err)
	}
	if count > 0 {
		return nil
	}

	items := []models.ExtraItem{
		{ID: "1", Name: "加購清潔劑", Price: 200},
		{ID: "2", Name: "高樓層費", Price: 500},
	}
	return db.Create(&items).Error
}

// seedZones 初始化服务区域，供自动派工和区域管理页面使用。
func seedZones(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.ServiceZone{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count zones: %w", err)
	}
	if count > 0 {
		return nil
	}

	zones := []models.ServiceZone{
		{
			ID:                    "zone-1",
			Name:                  "台北市信義/大安/中山",
			Districts:             jsonArray([]string{"信義區", "大安區", "中山區", "松山區", "中正區"}),
			AssignedTechnicianIDs: jsonArray([]uint{2, 4}),
		},
		{
			ID:                    "zone-2",
			Name:                  "新北市板橋/中和/永和",
			Districts:             jsonArray([]string{"板橋區", "中和區", "永和區", "新店區", "土城區"}),
			AssignedTechnicianIDs: jsonArray([]uint{3}),
		},
	}
	return db.Create(&zones).Error
}

// seedCustomers 初始化客户列表，避免管理页和新建预约联调时没有基础数据。
func seedCustomers(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.Customer{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count customers: %w", err)
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UTC()
	joined1 := now.AddDate(0, -4, 0)
	joined2 := now.AddDate(0, -3, -5)

	customers := []models.Customer{
		{
			ID:           "0922333444",
			Name:         "張先生",
			Phone:        "0922333444",
			Address:      "台北市信義區信義路五段7號",
			LineName:     strPtr("Kevin Chang"),
			LineUID:      strPtr("U123456789"),
			LinePicture:  strPtr("https://api.dicebear.com/7.x/avataaars/svg?seed=Kevin"),
			LineJoinedAt: &joined1,
			LineData:     jsonRaw(`{}`),
		},
		{
			ID:           "0933444555",
			Name:         "林小姐",
			Phone:        "0933444555",
			Address:      "新北市板橋區文化路一段",
			LineName:     strPtr("林小美"),
			LineUID:      strPtr("U987654321"),
			LinePicture:  strPtr("https://api.dicebear.com/7.x/avataaars/svg?seed=Lin"),
			LineJoinedAt: &joined2,
			LineData:     jsonRaw(`{}`),
		},
		{ID: "0955666777", Name: "陳先生", Phone: "0955666777", Address: "台北市大安區忠孝東路", LineData: jsonRaw(`{}`)},
		{ID: "0966777888", Name: "黃太太", Phone: "0966777888", Address: "台北市中山區南京東路三段", LineData: jsonRaw(`{}`)},
		{ID: "0977888999", Name: "曾先生", Phone: "0977888999", Address: "新北市中和區景安路", LineData: jsonRaw(`{}`)},
	}
	return db.Create(&customers).Error
}

// seedAppointments 初始化多种状态的预约单，覆盖列表、排程、回访和财务等页面。
// seedAppointments 初始化多种状态的预约单，覆盖列表、排程、回访和财务等页面。
func seedAppointments(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.Appointment{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count appointments: %w", err)
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UTC()
	today10 := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, time.UTC)
	today14 := time.Date(now.Year(), now.Month(), now.Day(), 14, 0, 0, 0, time.UTC)
	today16 := time.Date(now.Year(), now.Month(), now.Day(), 16, 0, 0, 0, time.UTC)
	yesterday9 := today10.AddDate(0, 0, -1).Add(-1 * time.Hour)
	yesterday15 := today14.AddDate(0, 0, -1).Add(1 * time.Hour)
	lastWeek11 := today10.AddDate(0, 0, -6).Add(1 * time.Hour)

	appointments := []models.Appointment{
		{
			ID:             1,
			CustomerName:   "張先生",
			Address:        "台北市信義區信義路五段7號",
			Phone:          "0922333444",
			Items:          jsonRaw(`[{"id":"1","type":"分離式","note":"客廳","price":2500},{"id":"2","type":"分離式","note":"主臥","price":2500}]`),
			ExtraItems:     jsonRaw(`[]`),
			PaymentMethod:  "現金",
			TotalAmount:    5000,
			ScheduledAt:    today10,
			ScheduledEnd:   timePtr(today10.Add(2 * time.Hour)),
			Status:         "assigned",
			TechnicianID:   uintPtr(2),
			TechnicianName: strPtr("王師傅"),
			Photos:         jsonRaw(`[]`),
			ZoneID:         strPtr("zone-1"),
		},
		{
			ID:            2,
			CustomerName:  "林小姐",
			Address:       "新北市板橋區文化路一段",
			Phone:         "0933444555",
			Items:         jsonRaw(`[{"id":"3","type":"吊隱式","note":"全室","price":3500}]`),
			ExtraItems:    jsonRaw(`[{"id":"e1","name":"室外機清洗","price":500}]`),
			PaymentMethod: "轉帳",
			TotalAmount:   4000,
			ScheduledAt:   today14,
			ScheduledEnd:  timePtr(today14.Add(90 * time.Minute)),
			Status:        "pending",
			Photos:        jsonRaw(`[]`),
			ZoneID:        strPtr("zone-2"),
			LineUID:       strPtr("U987654321"),
		},
		{
			ID:              3,
			CustomerName:    "陳先生",
			Address:         "台北市大安區忠孝東路",
			Phone:           "0955666777",
			Items:           jsonRaw(`[{"id":"4","type":"窗型","note":"書房","price":1800}]`),
			ExtraItems:      jsonRaw(`[]`),
			PaymentMethod:   "現金",
			TotalAmount:     1800,
			PaidAmount:      1800,
			ScheduledAt:     yesterday9,
			ScheduledEnd:    timePtr(yesterday9.Add(1 * time.Hour)),
			Status:          "completed",
			TechnicianID:    uintPtr(3),
			TechnicianName:  strPtr("李師傅"),
			Photos:          jsonRaw(`["https://images.unsplash.com/photo-1581094288338-2314dddb79a1?w=400"]`),
			PaymentReceived: true,
			SignatureData:   strPtr("data:image/svg+xml;base64,PHN2Zy8+"),
			CheckinTime:     timePtr(yesterday9.Add(5 * time.Minute)),
			CheckoutTime:    timePtr(yesterday9.Add(55 * time.Minute)),
			CompletedTime:   timePtr(yesterday9.Add(55 * time.Minute)),
			CreatedAt:       yesterday9.Add(-24 * time.Hour),
			ZoneID:          strPtr("zone-1"),
		},
		{
			ID:             4,
			CustomerName:   "黃太太",
			Address:        "台北市中山區南京東路三段",
			Phone:          "0966777888",
			Items:          jsonRaw(`[{"id":"5","type":"分離式","note":"客廳","price":2500}]`),
			ExtraItems:     jsonRaw(`[]`),
			PaymentMethod:  "轉帳",
			TotalAmount:    2500,
			ScheduledAt:    today16,
			ScheduledEnd:   timePtr(today16.Add(1 * time.Hour)),
			Status:         "assigned",
			TechnicianID:   uintPtr(4),
			TechnicianName: strPtr("陳師傅"),
			Photos:         jsonRaw(`[]`),
			ZoneID:         strPtr("zone-1"),
		},
		{
			ID:            5,
			CustomerName:  "曾先生",
			Address:       "新北市中和區景安路",
			Phone:         "0977888999",
			Items:         jsonRaw(`[{"id":"6","type":"分離式","note":"主臥","price":2500}]`),
			ExtraItems:    jsonRaw(`[{"id":"e2","name":"高樓層費","price":500}]`),
			PaymentMethod: "現金",
			TotalAmount:   3000,
			ScheduledAt:   today10.AddDate(0, 0, 1),
			ScheduledEnd:  timePtr(today10.AddDate(0, 0, 1).Add(1 * time.Hour)),
			Status:        "pending",
			Photos:        jsonRaw(`[]`),
			ZoneID:        strPtr("zone-2"),
		},
		{
			ID:              6,
			CustomerName:    "林先生",
			Address:         "台北市信義區忠孝東路五段",
			Phone:           "0912888777",
			Items:           jsonRaw(`[{"id":"7","type":"分離式","note":"客廳","price":2500}]`),
			ExtraItems:      jsonRaw(`[]`),
			PaymentMethod:   "現金",
			TotalAmount:     2500,
			PaidAmount:      2500,
			ScheduledAt:     yesterday15,
			ScheduledEnd:    timePtr(yesterday15.Add(1 * time.Hour)),
			Status:          "completed",
			TechnicianID:    uintPtr(2),
			TechnicianName:  strPtr("王師傅"),
			Photos:          jsonRaw(`["https://images.unsplash.com/photo-1585771724684-38269d6639fd?w=400"]`),
			PaymentReceived: true,
			SignatureData:   strPtr("data:image/svg+xml;base64,PHN2Zy8+"),
			CheckinTime:     timePtr(yesterday15.Add(10 * time.Minute)),
			CheckoutTime:    timePtr(yesterday15.Add(70 * time.Minute)),
			CompletedTime:   timePtr(yesterday15.Add(70 * time.Minute)),
			CreatedAt:       lastWeek11,
			ZoneID:          strPtr("zone-1"),
		},
	}

	return db.Create(&appointments).Error
}

// seedCashLedgerEntries 初始化人工回款流水，搭配前端自动收款推导一起工作。
func seedCashLedgerEntries(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.CashLedgerEntry{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count cash ledger: %w", err)
	}
	if count > 0 {
		return nil
	}

	entries := []models.CashLedgerEntry{
		{
			ID:            "cl-1",
			TechnicianID:  3,
			AppointmentID: uintPtr(3),
			Type:          "collect",
			Amount:        1800,
			Note:          "陳先生冷氣清洗 - 現金收款",
			CreatedAt:     time.Now().UTC().Add(-48 * time.Hour),
		},
	}
	return db.Create(&entries).Error
}

// seedReviews 初始化评价数据，支撑评价列表与客户画像。
func seedReviews(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.Review{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count reviews: %w", err)
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UTC()
	reviews := []models.Review{
		{ID: "rev-1", AppointmentID: 3, CustomerName: "陳先生", TechnicianID: uintPtr(3), TechnicianName: strPtr("李師傅"), Rating: 5, Misconducts: jsonArray([]string{}), Comment: "服務很專業，冷氣清洗得很乾淨！", SharedLine: false, CreatedAt: now.Add(-24 * time.Hour)},
		{ID: "rev-2", AppointmentID: 6, CustomerName: "林先生", TechnicianID: uintPtr(2), TechnicianName: strPtr("王師傅"), Rating: 3, Misconducts: jsonArray([]string{"late_arrival", "not_clean"}), Comment: "師傅遲到了半小時，清洗後還是有點髒", SharedLine: false, CreatedAt: now.Add(-48 * time.Hour)},
	}
	return db.Create(&reviews).Error
}

// seedNotificationLogs 初始化通知纪录，便于通知面板联调。
func seedNotificationLogs(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.NotificationLog{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count notification logs: %w", err)
	}
	if count > 0 {
		return nil
	}

	return db.Create(&[]models.NotificationLog{
		{
			ID:            "notif-1",
			AppointmentID: 1,
			Type:          "line",
			Message:       "您好 張先生，您的冷氣清洗已預約在今日 10:00，師傅 王師傅 將為您服務。",
			SentAt:        time.Now().UTC().Add(-2 * time.Hour),
		},
	}).Error
}

// seedLineFriends 初始化 LINE 好友资料，供 LINE 纪录和预约关联器使用。
func seedLineFriends(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.LineFriend{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count line friends: %w", err)
	}
	if count > 0 {
		return nil
	}

	parse := func(v string) time.Time {
		t, _ := time.Parse(time.RFC3339, v)
		return t
	}
	phone1 := "0922333444"
	phone2 := "0933444555"
	customer1 := "0922333444"
	customer2 := "0933444555"

	items := []models.LineFriend{
		{LineUID: "U123456789", LineName: "Kevin Chang", LinePicture: "https://api.dicebear.com/7.x/avataaars/svg?seed=Kevin", JoinedAt: parse("2025-11-20T08:30:00.000Z"), Phone: &phone1, LinkedCustomerID: &customer1, Status: "followed", LastPayload: datatypes.JSON([]byte(`{}`))},
		{LineUID: "U987654321", LineName: "林小美", LinePicture: "https://api.dicebear.com/7.x/avataaars/svg?seed=Lin", JoinedAt: parse("2025-12-05T14:20:00.000Z"), Phone: &phone2, LinkedCustomerID: &customer2, Status: "followed", LastPayload: datatypes.JSON([]byte(`{}`))},
		{LineUID: "Uabc111222", LineName: "小花花", LinePicture: "https://api.dicebear.com/7.x/avataaars/svg?seed=Flower", JoinedAt: parse("2026-01-15T09:00:00.000Z"), Status: "followed", LastPayload: datatypes.JSON([]byte(`{}`))},
		{LineUID: "Udef333444", LineName: "David Wu", LinePicture: "https://api.dicebear.com/7.x/avataaars/svg?seed=David", JoinedAt: parse("2026-02-01T16:45:00.000Z"), Status: "followed", LastPayload: datatypes.JSON([]byte(`{}`))},
		{LineUID: "Ughi555666", LineName: "阿明的冷氣", LinePicture: "https://api.dicebear.com/7.x/avataaars/svg?seed=Ming", JoinedAt: parse("2026-02-28T11:10:00.000Z"), Status: "followed", LastPayload: datatypes.JSON([]byte(`{}`))},
		{LineUID: "Ujkl777888", LineName: "陳太太", LinePicture: "https://api.dicebear.com/7.x/avataaars/svg?seed=ChenTai", JoinedAt: parse("2026-03-01T07:55:00.000Z"), Status: "followed", LastPayload: datatypes.JSON([]byte(`{}`))},
	}

	return db.Create(&items).Error
}

// jsonArray 把任意 Go 值序列化为 JSON，供种子数据快速构建 jsonb 字段。
func jsonArray[T any](value T) datatypes.JSON {
	raw, _ := json.Marshal(value)
	return datatypes.JSON(raw)
}

// jsonRaw 直接把原始 JSON 字符串包装为 datatypes.JSON，适合内联静态示例数据。
func jsonRaw(raw string) datatypes.JSON {
	return datatypes.JSON([]byte(raw))
}

// strPtr 返回字符串指针，便于内联构建可选字段。
func strPtr(value string) *string {
	return &value
}

// uintPtr 返回 uint 指针，便于内联构建可选关联字段。
func uintPtr(value uint) *uint {
	return &value
}

// timePtr 返回时间指针，便于内联构建可选时间字段。
func timePtr(value time.Time) *time.Time {
	return &value
}

// ParseReminderDays 统一把设置表里的字符串值解析为整数，避免多处重复转换。
func ParseReminderDays(raw string) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 180
	}
	return value
}
