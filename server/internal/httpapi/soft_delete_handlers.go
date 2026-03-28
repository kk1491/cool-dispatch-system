package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cool-dispatch/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	deletedResourceAppointments = "appointments"
	deletedResourceCustomers    = "customers"
	deletedResourceTechnicians  = "technicians"
	deletedResourceZones        = "zones"
	deletedResourceServiceItems = "service-items"
	deletedResourceExtraItems   = "extra-items"
)

// deletedResourceItem 是回收站列表的统一读模型，便于前端按类型展示与批量选择。
type deletedResourceItem struct {
	// ID 是资源主键，统一转成字符串后前端可直接作为选中值使用。
	ID string `json:"id"`
	// PrimaryText 是列表主标题。
	PrimaryText string `json:"primary_text"`
	// SecondaryText 是辅助说明，帮助管理员判断是否恢复。
	SecondaryText string `json:"secondary_text,omitempty"`
	// DeletedAt 是进入回收站时间。
	DeletedAt time.Time `json:"deleted_at"`
}

// deletedResourcesPayload 聚合六类可恢复业务数据，避免前端重复调多条回收站接口。
type deletedResourcesPayload struct {
	Appointments []deletedResourceItem `json:"appointments"`
	Customers    []deletedResourceItem `json:"customers"`
	Technicians  []deletedResourceItem `json:"technicians"`
	Zones        []deletedResourceItem `json:"zones"`
	ServiceItems []deletedResourceItem `json:"service_items"`
	ExtraItems   []deletedResourceItem `json:"extra_items"`
}

type restoreDeletedResourcesPayload struct {
	Resource string   `json:"resource"`
	IDs      []string `json:"ids"`
}

type deletedResourceActionError struct {
	status  int
	message string
}

func (e *deletedResourceActionError) Error() string {
	return e.message
}

// GetDeletedResources 返回当前回收站中的全部业务数据，供前端管理恢复使用。
func (h *Handler) GetDeletedResources(c *gin.Context) {
	payload, err := h.buildDeletedResourcesPayload()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "載入回收站資料失敗")
		return
	}
	respondData(c, http.StatusOK, "success", payload)
}

// RestoreDeletedResources 批量恢复指定类型的软删除数据。
func (h *Handler) RestoreDeletedResources(c *gin.Context) {
	var payload restoreDeletedResourcesPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid restore deleted resources payload") {
		return
	}

	resource := strings.TrimSpace(payload.Resource)
	if resource == "" || len(payload.IDs) == 0 {
		respondMessage(c, http.StatusBadRequest, "恢復資料請提供資源類型與至少一筆 ID")
		return
	}

	restoredCount, err := h.restoreDeletedResources(resource, payload.IDs)
	if err != nil {
		var actionErr *deletedResourceActionError
		if errors.As(err, &actionErr) {
			respondMessage(c, actionErr.status, actionErr.message)
			return
		}
		respondMessage(c, http.StatusInternalServerError, "恢復回收站資料失敗")
		return
	}

	respondData(c, http.StatusOK, "success", gin.H{
		"resource":       resource,
		"restored_count": restoredCount,
	})
}

func (h *Handler) buildDeletedResourcesPayload() (deletedResourcesPayload, error) {
	payload := deletedResourcesPayload{}

	var appointments []models.Appointment
	if err := h.db.Unscoped().
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Find(&appointments).Error; err != nil {
		return payload, err
	}
	for _, appointment := range appointments {
		payload.Appointments = append(payload.Appointments, deletedResourceItem{
			ID:            strconv.FormatUint(uint64(appointment.ID), 10),
			PrimaryText:   fmt.Sprintf("預約 #%d %s", appointment.ID, appointment.CustomerName),
			SecondaryText: fmt.Sprintf("%s｜%s", appointment.ScheduledAt.Format("2006/01/02 15:04"), appointment.Address),
			DeletedAt:     appointment.DeletedAt.Time,
		})
	}

	var customers []models.Customer
	if err := h.db.Unscoped().
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Find(&customers).Error; err != nil {
		return payload, err
	}
	for _, customer := range customers {
		payload.Customers = append(payload.Customers, deletedResourceItem{
			ID:            customer.ID,
			PrimaryText:   customer.Name,
			SecondaryText: fmt.Sprintf("%s｜%s", customer.Phone, customer.Address),
			DeletedAt:     customer.DeletedAt.Time,
		})
	}

	var technicians []models.User
	if err := h.db.Unscoped().
		Where("role = ? AND deleted_at IS NOT NULL", "technician").
		Order("deleted_at desc").
		Find(&technicians).Error; err != nil {
		return payload, err
	}
	for _, technician := range technicians {
		payload.Technicians = append(payload.Technicians, deletedResourceItem{
			ID:            strconv.FormatUint(uint64(technician.ID), 10),
			PrimaryText:   technician.Name,
			SecondaryText: technician.Phone,
			DeletedAt:     technician.DeletedAt.Time,
		})
	}

	var zones []models.ServiceZone
	if err := h.db.Unscoped().
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Find(&zones).Error; err != nil {
		return payload, err
	}
	for _, zone := range zones {
		payload.Zones = append(payload.Zones, deletedResourceItem{
			ID:            zone.ID,
			PrimaryText:   zone.Name,
			SecondaryText: "服務區域",
			DeletedAt:     zone.DeletedAt.Time,
		})
	}

	var serviceItems []models.ServiceItem
	if err := h.db.Unscoped().
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Find(&serviceItems).Error; err != nil {
		return payload, err
	}
	for _, item := range serviceItems {
		payload.ServiceItems = append(payload.ServiceItems, deletedResourceItem{
			ID:            item.ID,
			PrimaryText:   item.Name,
			SecondaryText: fmt.Sprintf("預設金額 NT$ %d", item.DefaultPrice),
			DeletedAt:     item.DeletedAt.Time,
		})
	}

	var extraItems []models.ExtraItem
	if err := h.db.Unscoped().
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Find(&extraItems).Error; err != nil {
		return payload, err
	}
	for _, item := range extraItems {
		payload.ExtraItems = append(payload.ExtraItems, deletedResourceItem{
			ID:            item.ID,
			PrimaryText:   item.Name,
			SecondaryText: fmt.Sprintf("金額 NT$ %d", item.Price),
			DeletedAt:     item.DeletedAt.Time,
		})
	}

	return payload, nil
}

func (h *Handler) restoreDeletedResources(resource string, ids []string) (int64, error) {
	switch resource {
	case deletedResourceAppointments:
		return restoreAppointments(h.db, ids)
	case deletedResourceCustomers:
		return restoreCustomers(h.db, ids)
	case deletedResourceTechnicians:
		return restoreTechnicians(h.db, ids)
	case deletedResourceZones:
		return restoreStringKeyResource[models.ServiceZone](h.db, ids)
	case deletedResourceServiceItems:
		return restoreStringKeyResource[models.ServiceItem](h.db, ids)
	case deletedResourceExtraItems:
		return restoreStringKeyResource[models.ExtraItem](h.db, ids)
	default:
		return 0, &deletedResourceActionError{status: http.StatusBadRequest, message: "不支援的回收站資源類型"}
	}
}

func restoreAppointments(db *gorm.DB, ids []string) (int64, error) {
	uintIDs, err := parseUintIDs(ids)
	if err != nil {
		return 0, &deletedResourceActionError{status: http.StatusBadRequest, message: "恢復資料 ID 格式錯誤"}
	}
	result := db.Unscoped().
		Model(&models.Appointment{}).
		Where("id IN ? AND deleted_at IS NOT NULL", uintIDs).
		Update("deleted_at", nil)
	return result.RowsAffected, result.Error
}

func restoreTechnicians(db *gorm.DB, ids []string) (int64, error) {
	uintIDs, err := parseUintIDs(ids)
	if err != nil {
		return 0, &deletedResourceActionError{status: http.StatusBadRequest, message: "恢復資料 ID 格式錯誤"}
	}
	result := db.Unscoped().
		Model(&models.User{}).
		Where("role = ? AND id IN ? AND deleted_at IS NOT NULL", "technician", uintIDs).
		Update("deleted_at", nil)
	return result.RowsAffected, result.Error
}

func restoreCustomers(db *gorm.DB, ids []string) (int64, error) {
	trimmedIDs := normalizeStringIDs(ids)
	if len(trimmedIDs) == 0 {
		return 0, &deletedResourceActionError{status: http.StatusBadRequest, message: "恢復資料請至少提供一筆 ID"}
	}

	var restoredCount int64
	err := db.Transaction(func(tx *gorm.DB) error {
		var customers []models.Customer
		if err := tx.Unscoped().
			Where("id IN ? AND deleted_at IS NOT NULL", trimmedIDs).
			Find(&customers).Error; err != nil {
			return err
		}

		for _, customer := range customers {
			if result := tx.Unscoped().
				Model(&models.Customer{}).
				Where("id = ? AND deleted_at IS NOT NULL", customer.ID).
				Update("deleted_at", nil); result.Error != nil {
				return result.Error
			} else {
				restoredCount += result.RowsAffected
			}

			if customer.LineUID != nil && strings.TrimSpace(*customer.LineUID) != "" {
				if err := clearCustomerLineFieldsByLineUID(tx, *customer.LineUID, stringPtr(customer.ID)); err != nil {
					return err
				}
				if err := tx.Model(&models.LineFriend{}).
					Where("line_uid = ?", strings.TrimSpace(*customer.LineUID)).
					Update("linked_customer_id", customer.ID).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	return restoredCount, err
}

func restoreStringKeyResource[T any](db *gorm.DB, ids []string) (int64, error) {
	trimmedIDs := normalizeStringIDs(ids)
	if len(trimmedIDs) == 0 {
		return 0, &deletedResourceActionError{status: http.StatusBadRequest, message: "恢復資料請至少提供一筆 ID"}
	}
	result := db.Unscoped().
		Model(new(T)).
		Where("id IN ? AND deleted_at IS NOT NULL", trimmedIDs).
		Update("deleted_at", nil)
	return result.RowsAffected, result.Error
}

func parseUintIDs(ids []string) ([]uint, error) {
	result := make([]uint, 0, len(ids))
	for _, raw := range ids {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		parsed, err := strconv.ParseUint(trimmed, 10, 64)
		if err != nil {
			return nil, err
		}
		result = append(result, uint(parsed))
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("restore ids must not be empty")
	}
	return result, nil
}

func normalizeStringIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, raw := range ids {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
