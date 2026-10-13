package service

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
)

type Dashboard struct{ store DashboardRepository }

func NewDashboard(store DashboardRepository) *Dashboard { return &Dashboard{store} }

func dashboardSearch(query string) (string, error) {
	if !utf8.ValidString(query) {
		return "", domain.ErrInvalid
	}
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > 200 {
		return "", domain.ErrInvalid
	}
	for _, r := range query {
		if unicode.IsControl(r) {
			return "", domain.ErrInvalid
		}
	}
	return query, nil
}

func (s *Dashboard) PresenceStudents(ctx context.Context, actor string, filter dto.PresenceFilter) (domain.PresencePage, error) {
	if (filter.Status != "" && filter.Status != "IN" && filter.Status != "OUT" && filter.Status != "UNKNOWN") || filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 || filter.Offset > 1000000 {
		return domain.PresencePage{}, domain.ErrInvalid
	}
	query, err := dashboardSearch(filter.Query)
	if err != nil {
		return domain.PresencePage{}, err
	}
	filter.Query = query
	return s.store.PresenceStudents(ctx, actor, filter)
}

func (s *Dashboard) PresenceSummary(ctx context.Context, actor, query string) (domain.PresenceSummary, error) {
	query, err := dashboardSearch(query)
	if err != nil {
		return domain.PresenceSummary{}, err
	}
	return s.store.PresenceSummary(ctx, actor, query)
}
