package service

import (
	"context"
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/invoiceapplication"
	"github.com/Wei-Shaw/sub2api/ent/invoiceapplicationitem"
	"github.com/Wei-Shaw/sub2api/ent/invoicehistoricalmark"
	"github.com/Wei-Shaw/sub2api/ent/invoiceprofile"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	InvoiceApplicationStatusPending   = "PENDING"
	InvoiceApplicationStatusCompleted = "COMPLETED"
	InvoiceApplicationStatusRejected  = "REJECTED"
	InvoiceRecordStatusHistorical     = "HISTORICAL_COMPLETED"

	InvoiceSourcePaymentOrder      = "payment_order"
	InvoiceSourceRedeemCode        = "redeem_code"
	InvoiceSourceAdminGrant        = "admin_grant"
	InvoiceSourceAffiliateTransfer = "affiliate_transfer"

	invoiceMinimumAmount = 1000
)

// InvoiceProfileData is the latest invoice information saved for a user.
type InvoiceProfileData struct {
	Email       string `json:"email"`
	TaxNumber   string `json:"tax_number"`
	CompanyName string `json:"company_name"`
}

// InvoiceSourceRef identifies a recharge record selected by the user.
type InvoiceSourceRef struct {
	SourceType string `json:"source_type"`
	SourceID   int64  `json:"source_id"`
}

type CreateInvoiceApplicationInput struct {
	InvoiceProfileData
	Sources []InvoiceSourceRef `json:"sources"`
}

// InvoiceRecord is a normalized payment, redemption, grant, or affiliate transfer.
// Only records marked Selectable can be included in an invoice application.
type InvoiceRecord struct {
	UserID            int64      `json:"user_id,omitempty"`
	UserEmail         string     `json:"user_email,omitempty"`
	UserName          string     `json:"user_name,omitempty"`
	SourceType        string     `json:"source_type"`
	SourceID          int64      `json:"source_id"`
	SourceReference   string     `json:"source_reference"`
	Amount            float64    `json:"amount"`
	OccurredAt        time.Time  `json:"occurred_at"`
	Selectable        bool       `json:"selectable"`
	IneligibleReason  string     `json:"ineligible_reason,omitempty"`
	ApplicationNo     string     `json:"application_no,omitempty"`
	ApplicationStatus string     `json:"application_status,omitempty"`
	MarkedAt          *time.Time `json:"marked_at,omitempty"`
	MarkedBy          *int64     `json:"marked_by,omitempty"`
	MarkedByEmail     string     `json:"marked_by_email,omitempty"`
}

type InvoiceApplicationItem struct {
	SourceType      string  `json:"source_type"`
	SourceID        int64   `json:"source_id"`
	SourceReference string  `json:"source_reference"`
	Amount          float64 `json:"amount"`
}

type InvoiceApplication struct {
	ID              int64                    `json:"id"`
	ApplicationNo   string                   `json:"application_no"`
	UserID          int64                    `json:"user_id"`
	UserEmail       string                   `json:"user_email,omitempty"`
	Email           string                   `json:"email"`
	TaxNumber       string                   `json:"tax_number"`
	CompanyName     string                   `json:"company_name"`
	TotalAmount     float64                  `json:"total_amount"`
	Status          string                   `json:"status"`
	RejectionReason string                   `json:"rejection_reason,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
	CompletedAt     *time.Time               `json:"completed_at,omitempty"`
	CompletedBy     *int64                   `json:"completed_by,omitempty"`
	RejectedAt      *time.Time               `json:"rejected_at,omitempty"`
	RejectedBy      *int64                   `json:"rejected_by,omitempty"`
	Items           []InvoiceApplicationItem `json:"items,omitempty"`
}

type InvoiceApplicationListParams struct {
	Page     int
	PageSize int
	Status   string
	Keyword  string
}

type InvoiceRecordListParams struct {
	Page          int
	PageSize      int
	UserID        int64
	Keyword       string
	InvoiceStatus string
}

type invoiceApplicationBinding struct {
	ApplicationNo string
	Status        string
	MarkedAt      *time.Time
	MarkedBy      *int64
	MarkedByEmail string
}

type resolvedInvoiceSource struct {
	UserID int64
	Item   InvoiceApplicationItem
}

// InvoiceService owns invoice eligibility, immutable application snapshots, and completion state.
type InvoiceService struct {
	entClient      *dbent.Client
	settingService *SettingService
}

func NewInvoiceService(entClient *dbent.Client, settingService *SettingService) *InvoiceService {
	return &InvoiceService{entClient: entClient, settingService: settingService}
}

var ErrEnterpriseInvoiceDisabled = infraerrors.NotFound("INVOICE_DISABLED", "enterprise invoicing is currently disabled")

func (s *InvoiceService) ensureInvoiceEnabled(ctx context.Context) error {
	if s == nil || s.settingService == nil || !s.settingService.IsEnterpriseInvoiceEnabled(ctx) {
		return ErrEnterpriseInvoiceDisabled
	}
	return nil
}

func (s *InvoiceService) GetProfile(ctx context.Context, userID int64) (*InvoiceProfileData, error) {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return nil, err
	}
	profile, err := s.entClient.InvoiceProfile.Query().Where(invoiceprofile.UserIDEQ(userID)).Only(ctx)
	if dbent.IsNotFound(err) {
		return &InvoiceProfileData{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query invoice profile: %w", err)
	}
	return &InvoiceProfileData{Email: profile.Email, TaxNumber: profile.TaxNumber, CompanyName: profile.CompanyName}, nil
}

// ListUserRecords returns the user's complete recharge history in one normalized feed.
// Payment fulfillment's automatically created redeem codes are excluded to avoid double counting.
func (s *InvoiceService) ListUserRecords(ctx context.Context, userID int64, page, pageSize int) ([]InvoiceRecord, int64, error) {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return nil, 0, err
	}
	bindingBySource, err := s.loadSourceBindings(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.UserIDEQ(userID),
			paymentorder.OrderTypeEQ(payment.OrderTypeBalance),
			paymentorder.StatusEQ(OrderStatusCompleted),
			paymentorder.AmountGT(0),
		).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query invoice payment orders: %w", err)
	}

	linkedRechargeCodes := make(map[string]struct{}, len(orders))
	records := make([]InvoiceRecord, 0, len(orders)+8)
	for _, order := range orders {
		if order.RechargeCode != "" {
			linkedRechargeCodes[order.RechargeCode] = struct{}{}
		}
		occurredAt := order.CreatedAt
		if order.CompletedAt != nil {
			occurredAt = *order.CompletedAt
		}
		records = append(records, s.applySourceBinding(InvoiceRecord{
			SourceType:      InvoiceSourcePaymentOrder,
			SourceID:        order.ID,
			SourceReference: invoicePaymentReference(order),
			Amount:          order.Amount,
			OccurredAt:      occurredAt,
			Selectable:      true,
		}, bindingBySource))
	}

	redeemCodes, err := s.entClient.RedeemCode.Query().
		Where(
			redeemcode.UsedByEQ(userID),
			redeemcode.ValueGT(0),
			redeemcode.TypeIn(domain.RedeemTypeBalance, domain.AdjustmentTypeAdminBalance),
		).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query invoice redeem history: %w", err)
	}
	for _, code := range redeemCodes {
		if code.Type == domain.RedeemTypeBalance {
			if _, linked := linkedRechargeCodes[code.Code]; linked {
				continue
			}
			records = append(records, s.applySourceBinding(InvoiceRecord{
				SourceType:      InvoiceSourceRedeemCode,
				SourceID:        code.ID,
				SourceReference: code.Code,
				Amount:          code.Value,
				OccurredAt:      invoiceRedeemOccurredAt(code),
				Selectable:      true,
			}, bindingBySource))
			continue
		}

		records = append(records, InvoiceRecord{
			SourceType:       InvoiceSourceAdminGrant,
			SourceID:         code.ID,
			SourceReference:  code.Code,
			Amount:           code.Value,
			OccurredAt:       invoiceRedeemOccurredAt(code),
			Selectable:       false,
			IneligibleReason: "ADMIN_GRANT_EXCLUDED",
		})
	}

	affiliateRecords, err := s.listAffiliateTransferRecords(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	records = append(records, affiliateRecords...)

	sort.Slice(records, func(i, j int) bool {
		if records[i].OccurredAt.Equal(records[j].OccurredAt) {
			if records[i].SourceType == records[j].SourceType {
				return records[i].SourceID > records[j].SourceID
			}
			return records[i].SourceType < records[j].SourceType
		}
		return records[i].OccurredAt.After(records[j].OccurredAt)
	})

	total := int64(len(records))
	page, pageSize = normalizeInvoicePagination(page, pageSize)
	start := (page - 1) * pageSize
	if start >= len(records) {
		return []InvoiceRecord{}, total, nil
	}
	end := start + pageSize
	if end > len(records) {
		end = len(records)
	}
	return records[start:end], total, nil
}

func (s *InvoiceService) CreateApplication(ctx context.Context, userID int64, input CreateInvoiceApplicationInput) (*InvoiceApplication, error) {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return nil, err
	}
	profile, err := validateInvoiceProfile(input.InvoiceProfileData)
	if err != nil {
		return nil, err
	}
	refs, err := normalizeInvoiceSourceRefs(input.Sources)
	if err != nil {
		return nil, err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin invoice application transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockInvoiceSources(ctx, tx, refs); err != nil {
		return nil, err
	}
	items, total, err := s.resolveSelectedInvoiceItemsWithClient(ctx, tx.Client(), userID, refs)
	if err != nil {
		return nil, err
	}
	minimum := decimal.NewFromInt(invoiceMinimumAmount)
	if total.LessThan(minimum) {
		return nil, infraerrors.BadRequest("INVOICE_MINIMUM_NOT_MET", "selected recharge amount must be at least 1000.00").
			WithMetadata(map[string]string{"minimum_amount": minimum.StringFixed(2), "selected_amount": total.StringFixed(2)})
	}

	if err := tx.InvoiceProfile.Create().
		SetUserID(userID).
		SetEmail(profile.Email).
		SetTaxNumber(profile.TaxNumber).
		SetCompanyName(profile.CompanyName).
		OnConflictColumns(invoiceprofile.FieldUserID).
		UpdateNewValues().
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("upsert invoice profile: %w", err)
	}

	application, err := tx.InvoiceApplication.Create().
		SetUserID(userID).
		SetApplicationNo(newInvoiceApplicationNo()).
		SetEmail(profile.Email).
		SetTaxNumber(profile.TaxNumber).
		SetCompanyName(profile.CompanyName).
		SetTotalAmount(total.InexactFloat64()).
		SetStatus(InvoiceApplicationStatusPending).
		Save(ctx)
	if err != nil {
		if dbent.IsConstraintError(err) {
			return nil, infraerrors.Conflict("INVOICE_APPLICATION_CONFLICT", "invoice application could not be created, please retry")
		}
		return nil, fmt.Errorf("create invoice application: %w", err)
	}

	for _, item := range items {
		if _, err := tx.InvoiceApplicationItem.Create().
			SetApplicationID(application.ID).
			SetSourceType(item.SourceType).
			SetSourceID(item.SourceID).
			SetSourceReference(item.SourceReference).
			SetAmount(item.Amount).
			Save(ctx); err != nil {
			if dbent.IsConstraintError(err) {
				return nil, infraerrors.Conflict("INVOICE_SOURCE_ALREADY_APPLIED", "one or more selected recharge records have already been applied for invoice")
			}
			return nil, fmt.Errorf("create invoice application item: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit invoice application: %w", err)
	}
	return invoiceApplicationFromEntity(application, nil), nil
}

// ListHistoricalRecords returns all user recharge records that an administrator
// may audit for pre-launch invoicing. Grants, affiliate transfers, subscriptions,
// incomplete orders, and payment-linked redeem codes are deliberately omitted.
func (s *InvoiceService) ListHistoricalRecords(ctx context.Context, params InvoiceRecordListParams) ([]InvoiceRecord, int64, error) {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return nil, 0, err
	}
	status := strings.ToLower(strings.TrimSpace(params.InvoiceStatus))
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "available" && status != "applied" && status != "historical_completed" {
		return nil, 0, infraerrors.BadRequest("INVALID_INVOICE_RECORD_STATUS", "invalid invoice record status")
	}
	orderPredicates := []predicate.PaymentOrder{
		paymentorder.OrderTypeEQ(payment.OrderTypeBalance),
		paymentorder.StatusEQ(OrderStatusCompleted),
		paymentorder.AmountGT(0),
	}
	if params.UserID > 0 {
		orderPredicates = append(orderPredicates, paymentorder.UserIDEQ(params.UserID))
	}
	orders, err := s.entClient.PaymentOrder.Query().Where(orderPredicates...).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query historical payment records: %w", err)
	}
	codePredicates := []predicate.RedeemCode{
		redeemcode.UsedByNotNil(),
		redeemcode.ValueGT(0),
		redeemcode.TypeEQ(domain.RedeemTypeBalance),
	}
	if params.UserID > 0 {
		codePredicates = append(codePredicates, redeemcode.UsedByEQ(params.UserID))
	}
	codes, err := s.entClient.RedeemCode.Query().Where(codePredicates...).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query historical redeem records: %w", err)
	}
	codeValues := make([]string, 0, len(codes))
	for _, code := range codes {
		codeValues = append(codeValues, code.Code)
	}
	linkedRechargeCodes, err := s.linkedPaymentRechargeCodes(ctx, codeValues)
	if err != nil {
		return nil, 0, err
	}

	bindingBySource, err := s.loadAllSourceBindings(ctx)
	if err != nil {
		return nil, 0, err
	}
	userIDs := make([]int64, 0, len(orders)+len(codes))
	seenUsers := make(map[int64]struct{})
	for _, order := range orders {
		if _, seen := seenUsers[order.UserID]; !seen {
			seenUsers[order.UserID] = struct{}{}
			userIDs = append(userIDs, order.UserID)
		}
	}
	for _, code := range codes {
		if code.UsedBy != nil {
			if _, seen := seenUsers[*code.UsedBy]; !seen {
				seenUsers[*code.UsedBy] = struct{}{}
				userIDs = append(userIDs, *code.UsedBy)
			}
		}
	}
	usersByID := make(map[int64]*dbent.User, len(userIDs))
	if len(userIDs) > 0 {
		users, err := s.entClient.User.Query().Where(user.IDIn(userIDs...)).All(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("query invoice record users: %w", err)
		}
		for _, item := range users {
			usersByID[item.ID] = item
		}
		markedByIDs := make([]int64, 0)
		for _, binding := range bindingBySource {
			if binding.MarkedBy != nil {
				markedByIDs = append(markedByIDs, *binding.MarkedBy)
			}
		}
		if len(markedByIDs) > 0 {
			markedByUsers, err := s.entClient.User.Query().Where(user.IDIn(markedByIDs...)).All(ctx)
			if err != nil {
				return nil, 0, fmt.Errorf("query invoice operators: %w", err)
			}
			operatorEmail := make(map[int64]string, len(markedByUsers))
			for _, operator := range markedByUsers {
				operatorEmail[operator.ID] = operator.Email
			}
			for key, binding := range bindingBySource {
				if binding.MarkedBy != nil {
					binding.MarkedByEmail = operatorEmail[*binding.MarkedBy]
					bindingBySource[key] = binding
				}
			}
		}
	}

	records := make([]InvoiceRecord, 0, len(orders)+len(codes))
	for _, order := range orders {
		userEmail, userName := order.UserEmail, order.UserName
		if account := usersByID[order.UserID]; account != nil {
			userEmail, userName = account.Email, account.Username
		}
		record := InvoiceRecord{
			UserID: order.UserID, UserEmail: userEmail, UserName: userName,
			SourceType: InvoiceSourcePaymentOrder, SourceID: order.ID,
			SourceReference: invoicePaymentReference(order), Amount: order.Amount,
			OccurredAt: order.CreatedAt, Selectable: true,
		}
		if order.CompletedAt != nil {
			record.OccurredAt = *order.CompletedAt
		}
		record = s.applySourceBinding(record, bindingBySource)
		if includeAdminInvoiceRecord(record, status, params.Keyword) {
			records = append(records, record)
		}
	}
	for _, code := range codes {
		if linkedRechargeCodes[code.Code] || code.UsedBy == nil {
			continue
		}
		item := InvoiceRecord{
			UserID: *code.UsedBy, SourceType: InvoiceSourceRedeemCode, SourceID: code.ID,
			SourceReference: code.Code, Amount: code.Value, OccurredAt: invoiceRedeemOccurredAt(code), Selectable: true,
		}
		if account := usersByID[*code.UsedBy]; account != nil {
			item.UserEmail, item.UserName = account.Email, account.Username
		}
		item = s.applySourceBinding(item, bindingBySource)
		if includeAdminInvoiceRecord(item, status, params.Keyword) {
			records = append(records, item)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].OccurredAt.Equal(records[j].OccurredAt) {
			return invoiceSourceKey(records[i].SourceType, records[i].SourceID) > invoiceSourceKey(records[j].SourceType, records[j].SourceID)
		}
		return records[i].OccurredAt.After(records[j].OccurredAt)
	})
	total := int64(len(records))
	page, pageSize := normalizeInvoicePagination(params.Page, params.PageSize)
	start := (page - 1) * pageSize
	if start >= len(records) {
		return []InvoiceRecord{}, total, nil
	}
	end := start + pageSize
	if end > len(records) {
		end = len(records)
	}
	return records[start:end], total, nil
}

func (s *InvoiceService) MarkHistoricalRecords(ctx context.Context, adminID int64, refs []InvoiceSourceRef) (int, error) {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return 0, err
	}
	if adminID <= 0 {
		return 0, infraerrors.Unauthorized("UNAUTHORIZED", "administrator not authenticated")
	}
	normalized, err := normalizeInvoiceSourceRefs(refs)
	if err != nil {
		return 0, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin historical invoice transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockInvoiceSources(ctx, tx, normalized); err != nil {
		return 0, err
	}
	resolved, err := s.resolveHistoricalInvoiceSources(ctx, tx.Client(), normalized)
	if err != nil {
		return 0, err
	}
	for _, source := range resolved {
		_, err := tx.InvoiceHistoricalMark.Create().
			SetUserID(source.UserID).
			SetSourceType(source.Item.SourceType).
			SetSourceID(source.Item.SourceID).
			SetSourceReference(source.Item.SourceReference).
			SetAmount(source.Item.Amount).
			SetMarkedBy(adminID).
			Save(ctx)
		if err != nil {
			if dbent.IsConstraintError(err) {
				return 0, infraerrors.Conflict("INVOICE_SOURCE_ALREADY_APPLIED", "one or more selected recharge records have already been invoiced")
			}
			return 0, fmt.Errorf("create historical invoice mark: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit historical invoice marks: %w", err)
	}
	return len(resolved), nil
}

func includeAdminInvoiceRecord(record InvoiceRecord, status, keyword string) bool {
	recordStatus := "available"
	if record.ApplicationStatus == InvoiceRecordStatusHistorical {
		recordStatus = "historical_completed"
	} else if record.ApplicationStatus != "" {
		recordStatus = "applied"
	}
	if status != "all" && status != recordStatus {
		return false
	}
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return true
	}
	fields := []string{record.UserEmail, record.UserName, record.SourceReference, fmt.Sprintf("%d", record.UserID), fmt.Sprintf("%d", record.SourceID)}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), keyword) {
			return true
		}
	}
	return false
}

func (s *InvoiceService) ListApplications(ctx context.Context, params InvoiceApplicationListParams) ([]InvoiceApplication, int64, error) {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return nil, 0, err
	}
	q := s.entClient.InvoiceApplication.Query().WithUser()
	if params.Status != "" {
		if !isInvoiceApplicationStatus(params.Status) {
			return nil, 0, infraerrors.BadRequest("INVALID_INVOICE_STATUS", "invalid invoice application status")
		}
		q = q.Where(invoiceapplication.StatusEQ(params.Status))
	}
	keyword := strings.TrimSpace(params.Keyword)
	if keyword != "" {
		q = q.Where(invoiceapplication.Or(
			invoiceapplication.ApplicationNoContainsFold(keyword),
			invoiceapplication.EmailContainsFold(keyword),
			invoiceapplication.TaxNumberContainsFold(keyword),
			invoiceapplication.CompanyNameContainsFold(keyword),
		))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count invoice applications: %w", err)
	}
	page, pageSize := normalizeInvoicePagination(params.Page, params.PageSize)
	applications, err := q.Order(dbent.Desc(invoiceapplication.FieldCreatedAt)).Limit(pageSize).Offset((page - 1) * pageSize).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list invoice applications: %w", err)
	}
	out := make([]InvoiceApplication, 0, len(applications))
	for _, application := range applications {
		out = append(out, *invoiceApplicationFromEntity(application, nil))
	}
	return out, int64(total), nil
}

func (s *InvoiceService) ListApplicationsForUser(ctx context.Context, userID int64, page, pageSize int) ([]InvoiceApplication, int64, error) {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return nil, 0, err
	}
	q := s.entClient.InvoiceApplication.Query().Where(invoiceapplication.UserIDEQ(userID)).WithItems()
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count user invoice applications: %w", err)
	}
	page, pageSize = normalizeInvoicePagination(page, pageSize)
	applications, err := q.Order(dbent.Desc(invoiceapplication.FieldCreatedAt)).Limit(pageSize).Offset((page - 1) * pageSize).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list user invoice applications: %w", err)
	}
	out := make([]InvoiceApplication, 0, len(applications))
	for _, application := range applications {
		out = append(out, *invoiceApplicationFromEntity(application, application.Edges.Items))
	}
	return out, int64(total), nil
}

func (s *InvoiceService) GetApplication(ctx context.Context, applicationID int64) (*InvoiceApplication, error) {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return nil, err
	}
	application, err := s.entClient.InvoiceApplication.Query().
		Where(invoiceapplication.IDEQ(applicationID)).
		WithUser().
		WithItems().
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, infraerrors.NotFound("INVOICE_APPLICATION_NOT_FOUND", "invoice application not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get invoice application: %w", err)
	}
	return invoiceApplicationFromEntity(application, application.Edges.Items), nil
}

func (s *InvoiceService) CompleteApplication(ctx context.Context, applicationID, adminID int64) error {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return err
	}
	if adminID <= 0 {
		return infraerrors.Unauthorized("UNAUTHORIZED", "administrator not authenticated")
	}
	updated, err := s.entClient.InvoiceApplication.Update().
		Where(invoiceapplication.IDEQ(applicationID), invoiceapplication.StatusEQ(InvoiceApplicationStatusPending)).
		SetStatus(InvoiceApplicationStatusCompleted).
		SetCompletedAt(time.Now()).
		SetCompletedBy(adminID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("complete invoice application: %w", err)
	}
	if updated == 0 {
		return s.invoiceApplicationTransitionError(ctx, applicationID)
	}
	return nil
}

func (s *InvoiceService) RejectApplication(ctx context.Context, applicationID, adminID int64, reason string) error {
	if err := s.ensureInvoiceEnabled(ctx); err != nil {
		return err
	}
	if adminID <= 0 {
		return infraerrors.Unauthorized("UNAUTHORIZED", "administrator not authenticated")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 1000 {
		return infraerrors.BadRequest("INVALID_INVOICE_REJECTION_REASON", "rejection reason must be between 1 and 1000 characters")
	}
	updated, err := s.entClient.InvoiceApplication.Update().
		Where(invoiceapplication.IDEQ(applicationID), invoiceapplication.StatusEQ(InvoiceApplicationStatusPending)).
		SetStatus(InvoiceApplicationStatusRejected).
		SetRejectionReason(reason).
		SetRejectedAt(time.Now()).
		SetRejectedBy(adminID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("reject invoice application: %w", err)
	}
	if updated == 0 {
		return s.invoiceApplicationTransitionError(ctx, applicationID)
	}
	return nil
}

func (s *InvoiceService) resolveSelectedInvoiceItems(ctx context.Context, userID int64, refs []InvoiceSourceRef) ([]InvoiceApplicationItem, decimal.Decimal, error) {
	return s.resolveSelectedInvoiceItemsWithClient(ctx, s.entClient, userID, refs)
}

func (s *InvoiceService) resolveSelectedInvoiceItemsWithClient(ctx context.Context, client *dbent.Client, userID int64, refs []InvoiceSourceRef) ([]InvoiceApplicationItem, decimal.Decimal, error) {
	resolved, total, err := s.resolveEligibleInvoiceSources(ctx, client, &userID, refs)
	if err != nil {
		return nil, decimal.Zero, err
	}
	items := make([]InvoiceApplicationItem, 0, len(resolved))
	for _, source := range resolved {
		items = append(items, source.Item)
	}
	return items, total, nil
}

func (s *InvoiceService) resolveHistoricalInvoiceSources(ctx context.Context, client *dbent.Client, refs []InvoiceSourceRef) ([]resolvedInvoiceSource, error) {
	resolved, _, err := s.resolveEligibleInvoiceSources(ctx, client, nil, refs)
	return resolved, err
}

func (s *InvoiceService) resolveEligibleInvoiceSources(ctx context.Context, client *dbent.Client, userID *int64, refs []InvoiceSourceRef) ([]resolvedInvoiceSource, decimal.Decimal, error) {
	paymentIDs := make([]int64, 0, len(refs))
	redeemIDs := make([]int64, 0, len(refs))
	for _, ref := range refs {
		switch ref.SourceType {
		case InvoiceSourcePaymentOrder:
			paymentIDs = append(paymentIDs, ref.SourceID)
		case InvoiceSourceRedeemCode:
			redeemIDs = append(redeemIDs, ref.SourceID)
		default:
			return nil, decimal.Zero, infraerrors.BadRequest("INVALID_INVOICE_SOURCE", "selected record is not eligible for invoice")
		}
	}

	resolvedByKey := make(map[string]resolvedInvoiceSource, len(refs))
	if len(paymentIDs) > 0 {
		predicates := []predicate.PaymentOrder{paymentorder.IDIn(paymentIDs...), paymentorder.OrderTypeEQ(payment.OrderTypeBalance), paymentorder.StatusEQ(OrderStatusCompleted), paymentorder.AmountGT(0)}
		if userID != nil {
			predicates = append(predicates, paymentorder.UserIDEQ(*userID))
		}
		orders, err := client.PaymentOrder.Query().Where(predicates...).All(ctx)
		if err != nil {
			return nil, decimal.Zero, fmt.Errorf("resolve selected payment orders: %w", err)
		}
		for _, order := range orders {
			resolvedByKey[invoiceSourceKey(InvoiceSourcePaymentOrder, order.ID)] = resolvedInvoiceSource{
				UserID: order.UserID,
				Item:   InvoiceApplicationItem{SourceType: InvoiceSourcePaymentOrder, SourceID: order.ID, SourceReference: invoicePaymentReference(order), Amount: order.Amount},
			}
		}
	}
	if len(redeemIDs) > 0 {
		predicates := []predicate.RedeemCode{redeemcode.IDIn(redeemIDs...), redeemcode.TypeEQ(domain.RedeemTypeBalance), redeemcode.ValueGT(0), redeemcode.UsedByNotNil()}
		if userID != nil {
			predicates = append(predicates, redeemcode.UsedByEQ(*userID))
		}
		codes, err := client.RedeemCode.Query().Where(predicates...).All(ctx)
		if err != nil {
			return nil, decimal.Zero, fmt.Errorf("resolve selected redeem records: %w", err)
		}
		codesByValue := make([]string, 0, len(codes))
		for _, code := range codes {
			codesByValue = append(codesByValue, code.Code)
		}
		linkedCodes, err := s.linkedPaymentRechargeCodesWithClient(ctx, client, codesByValue)
		if err != nil {
			return nil, decimal.Zero, err
		}
		for _, code := range codes {
			if linkedCodes[code.Code] || code.UsedBy == nil {
				continue
			}
			resolvedByKey[invoiceSourceKey(InvoiceSourceRedeemCode, code.ID)] = resolvedInvoiceSource{
				UserID: *code.UsedBy,
				Item:   InvoiceApplicationItem{SourceType: InvoiceSourceRedeemCode, SourceID: code.ID, SourceReference: code.Code, Amount: code.Value},
			}
		}
	}
	resolved := make([]resolvedInvoiceSource, 0, len(refs))
	for _, ref := range refs {
		source, ok := resolvedByKey[invoiceSourceKey(ref.SourceType, ref.SourceID)]
		if !ok {
			return nil, decimal.Zero, infraerrors.Conflict("INVOICE_SOURCE_NOT_AVAILABLE", "one or more selected recharge records are no longer available for invoice")
		}
		resolved = append(resolved, source)
	}
	if err := ensureSourcesNotAppliedWithClient(ctx, client, refs); err != nil {
		return nil, decimal.Zero, err
	}
	total := decimal.Zero
	for _, source := range resolved {
		total = total.Add(decimal.NewFromFloat(source.Item.Amount))
	}
	return resolved, total.Round(2), nil
}

func ensureSourcesNotAppliedWithClient(ctx context.Context, client *dbent.Client, refs []InvoiceSourceRef) error {
	for _, ref := range refs {
		exists, err := client.InvoiceApplicationItem.Query().Where(
			invoiceapplicationitem.SourceTypeEQ(ref.SourceType),
			invoiceapplicationitem.SourceIDEQ(ref.SourceID),
		).Exist(ctx)
		if err != nil {
			return fmt.Errorf("check invoice source application status: %w", err)
		}
		if exists {
			return infraerrors.Conflict("INVOICE_SOURCE_ALREADY_APPLIED", "one or more selected recharge records have already been applied for invoice")
		}
		historical, err := client.InvoiceHistoricalMark.Query().Where(
			invoicehistoricalmark.SourceTypeEQ(ref.SourceType),
			invoicehistoricalmark.SourceIDEQ(ref.SourceID),
		).Exist(ctx)
		if err != nil {
			return fmt.Errorf("check historical invoice source status: %w", err)
		}
		if historical {
			return infraerrors.Conflict("INVOICE_SOURCE_ALREADY_APPLIED", "one or more selected recharge records have already been invoiced")
		}
	}
	return nil
}

func (s *InvoiceService) ensureSourcesNotApplied(ctx context.Context, refs []InvoiceSourceRef) error {
	return ensureSourcesNotAppliedWithClient(ctx, s.entClient, refs)
}

func (s *InvoiceService) linkedPaymentRechargeCodes(ctx context.Context, codes []string) (map[string]bool, error) {
	return s.linkedPaymentRechargeCodesWithClient(ctx, s.entClient, codes)
}

func (s *InvoiceService) linkedPaymentRechargeCodesWithClient(ctx context.Context, client *dbent.Client, codes []string) (map[string]bool, error) {
	linked := make(map[string]bool)
	if len(codes) == 0 {
		return linked, nil
	}
	orders, err := client.PaymentOrder.Query().Where(paymentorder.RechargeCodeIn(codes...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query linked payment recharge codes: %w", err)
	}
	for _, order := range orders {
		linked[order.RechargeCode] = true
	}
	return linked, nil
}

func lockInvoiceSources(ctx context.Context, tx *dbent.Tx, refs []InvoiceSourceRef) error {
	ordered := append([]InvoiceSourceRef(nil), refs...)
	sort.Slice(ordered, func(i, j int) bool {
		return invoiceSourceKey(ordered[i].SourceType, ordered[i].SourceID) < invoiceSourceKey(ordered[j].SourceType, ordered[j].SourceID)
	})
	for _, ref := range ordered {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, invoiceSourceKey(ref.SourceType, ref.SourceID)); err != nil {
			return fmt.Errorf("lock invoice source: %w", err)
		}
	}
	return nil
}

func (s *InvoiceService) loadSourceBindings(ctx context.Context, userID int64) (map[string]invoiceApplicationBinding, error) {
	items, err := s.entClient.InvoiceApplicationItem.Query().
		Where(invoiceapplicationitem.HasApplicationWith(invoiceapplication.UserIDEQ(userID))).
		WithApplication().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query applied invoice sources: %w", err)
	}
	bindings := make(map[string]invoiceApplicationBinding, len(items))
	for _, item := range items {
		if item.Edges.Application == nil {
			continue
		}
		bindings[invoiceSourceKey(item.SourceType, item.SourceID)] = invoiceApplicationBinding{
			ApplicationNo: item.Edges.Application.ApplicationNo,
			Status:        item.Edges.Application.Status,
		}
	}
	historical, err := s.entClient.InvoiceHistoricalMark.Query().
		Where(invoicehistoricalmark.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query historical invoice sources: %w", err)
	}
	for _, mark := range historical {
		bindings[invoiceSourceKey(mark.SourceType, mark.SourceID)] = invoiceApplicationBinding{
			Status: InvoiceRecordStatusHistorical,
		}
	}
	return bindings, nil
}

func (s *InvoiceService) loadAllSourceBindings(ctx context.Context) (map[string]invoiceApplicationBinding, error) {
	items, err := s.entClient.InvoiceApplicationItem.Query().WithApplication().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query applied invoice sources: %w", err)
	}
	bindings := make(map[string]invoiceApplicationBinding, len(items))
	for _, item := range items {
		if item.Edges.Application == nil {
			continue
		}
		bindings[invoiceSourceKey(item.SourceType, item.SourceID)] = invoiceApplicationBinding{
			ApplicationNo: item.Edges.Application.ApplicationNo,
			Status:        item.Edges.Application.Status,
		}
	}
	historical, err := s.entClient.InvoiceHistoricalMark.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query historical invoice sources: %w", err)
	}
	for _, mark := range historical {
		markedAt := mark.MarkedAt
		markedBy := mark.MarkedBy
		bindings[invoiceSourceKey(mark.SourceType, mark.SourceID)] = invoiceApplicationBinding{
			Status: InvoiceRecordStatusHistorical, MarkedAt: &markedAt, MarkedBy: &markedBy,
		}
	}
	return bindings, nil
}

func (s *InvoiceService) applySourceBinding(record InvoiceRecord, bindings map[string]invoiceApplicationBinding) InvoiceRecord {
	if binding, applied := bindings[invoiceSourceKey(record.SourceType, record.SourceID)]; applied {
		record.Selectable = false
		record.ApplicationNo = binding.ApplicationNo
		record.ApplicationStatus = binding.Status
		record.MarkedAt = binding.MarkedAt
		record.MarkedBy = binding.MarkedBy
		record.MarkedByEmail = binding.MarkedByEmail
		if binding.Status == InvoiceRecordStatusHistorical {
			record.IneligibleReason = "HISTORICAL_INVOICE_COMPLETED"
		}
	}
	return record
}

func (s *InvoiceService) listAffiliateTransferRecords(ctx context.Context, userID int64) ([]InvoiceRecord, error) {
	rows, err := s.entClient.QueryContext(ctx, `
SELECT id, amount, created_at
FROM user_affiliate_ledger
WHERE user_id = $1
  AND action = 'transfer'
  AND amount > 0
ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query affiliate transfer history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := make([]InvoiceRecord, 0)
	for rows.Next() {
		var (
			id        int64
			amount    float64
			createdAt time.Time
		)
		if err := rows.Scan(&id, &amount, &createdAt); err != nil {
			return nil, fmt.Errorf("scan affiliate transfer history: %w", err)
		}
		records = append(records, InvoiceRecord{
			SourceType:       InvoiceSourceAffiliateTransfer,
			SourceID:         id,
			SourceReference:  fmt.Sprintf("AFF-%d", id),
			Amount:           amount,
			OccurredAt:       createdAt,
			Selectable:       false,
			IneligibleReason: "AFFILIATE_TRANSFER_EXCLUDED",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read affiliate transfer history: %w", err)
	}
	return records, nil
}

func (s *InvoiceService) invoiceApplicationTransitionError(ctx context.Context, applicationID int64) error {
	exists, err := s.entClient.InvoiceApplication.Query().Where(invoiceapplication.IDEQ(applicationID)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check invoice application: %w", err)
	}
	if !exists {
		return infraerrors.NotFound("INVOICE_APPLICATION_NOT_FOUND", "invoice application not found")
	}
	return infraerrors.Conflict("INVOICE_APPLICATION_NOT_PENDING", "invoice application has already been processed")
}

func validateInvoiceProfile(profile InvoiceProfileData) (InvoiceProfileData, error) {
	profile.Email = strings.TrimSpace(profile.Email)
	profile.TaxNumber = strings.TrimSpace(profile.TaxNumber)
	profile.CompanyName = strings.TrimSpace(profile.CompanyName)
	if profile.Email == "" || profile.TaxNumber == "" || profile.CompanyName == "" {
		return InvoiceProfileData{}, infraerrors.BadRequest("INVOICE_PROFILE_REQUIRED", "email, tax number, and company name are required")
	}
	parsed, err := mail.ParseAddress(profile.Email)
	if err != nil || parsed.Address != profile.Email {
		return InvoiceProfileData{}, infraerrors.BadRequest("INVALID_INVOICE_EMAIL", "invalid invoice email address")
	}
	if len([]rune(profile.Email)) > 255 || len([]rune(profile.TaxNumber)) > 64 || len([]rune(profile.CompanyName)) > 255 {
		return InvoiceProfileData{}, infraerrors.BadRequest("INVALID_INVOICE_PROFILE", "invoice information exceeds the allowed length")
	}
	return profile, nil
}

func normalizeInvoiceSourceRefs(refs []InvoiceSourceRef) ([]InvoiceSourceRef, error) {
	if len(refs) == 0 || len(refs) > 100 {
		return nil, infraerrors.BadRequest("INVALID_INVOICE_SOURCES", "select between 1 and 100 recharge records")
	}
	seen := make(map[string]struct{}, len(refs))
	out := make([]InvoiceSourceRef, 0, len(refs))
	for _, ref := range refs {
		ref.SourceType = strings.TrimSpace(ref.SourceType)
		if ref.SourceID <= 0 || (ref.SourceType != InvoiceSourcePaymentOrder && ref.SourceType != InvoiceSourceRedeemCode) {
			return nil, infraerrors.BadRequest("INVALID_INVOICE_SOURCE", "selected record is not eligible for invoice")
		}
		key := invoiceSourceKey(ref.SourceType, ref.SourceID)
		if _, duplicated := seen[key]; duplicated {
			return nil, infraerrors.BadRequest("DUPLICATE_INVOICE_SOURCE", "selected recharge records must not contain duplicates")
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}
	return out, nil
}

func invoiceApplicationFromEntity(application *dbent.InvoiceApplication, items []*dbent.InvoiceApplicationItem) *InvoiceApplication {
	if application == nil {
		return nil
	}
	out := &InvoiceApplication{
		ID:            application.ID,
		ApplicationNo: application.ApplicationNo,
		UserID:        application.UserID,
		Email:         application.Email,
		TaxNumber:     application.TaxNumber,
		CompanyName:   application.CompanyName,
		TotalAmount:   application.TotalAmount,
		Status:        application.Status,
		CreatedAt:     application.CreatedAt,
		CompletedAt:   application.CompletedAt,
		CompletedBy:   application.CompletedBy,
		RejectedAt:    application.RejectedAt,
		RejectedBy:    application.RejectedBy,
	}
	if application.RejectionReason != nil {
		out.RejectionReason = *application.RejectionReason
	}
	if application.Edges.User != nil {
		out.UserEmail = application.Edges.User.Email
	}
	for _, item := range items {
		out.Items = append(out.Items, InvoiceApplicationItem{
			SourceType: item.SourceType, SourceID: item.SourceID,
			SourceReference: item.SourceReference, Amount: item.Amount,
		})
	}
	return out
}

func invoicePaymentReference(order *dbent.PaymentOrder) string {
	if order.OutTradeNo != "" {
		return order.OutTradeNo
	}
	return fmt.Sprintf("PAY-%d", order.ID)
}

func invoiceRedeemOccurredAt(code *dbent.RedeemCode) time.Time {
	if code.UsedAt != nil {
		return *code.UsedAt
	}
	return code.CreatedAt
}

func invoiceSourceKey(sourceType string, sourceID int64) string {
	return fmt.Sprintf("%s:%d", sourceType, sourceID)
}

func newInvoiceApplicationNo() string {
	return fmt.Sprintf("INV-%s-%s", time.Now().Format("20060102"), strings.ToUpper(uuid.NewString()[:8]))
}

func normalizeInvoicePagination(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func isInvoiceApplicationStatus(status string) bool {
	switch status {
	case InvoiceApplicationStatusPending, InvoiceApplicationStatusCompleted, InvoiceApplicationStatusRejected:
		return true
	default:
		return false
	}
}
