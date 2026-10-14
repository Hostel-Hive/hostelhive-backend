package service

import (
	"strings"

	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	sdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

func Normalize(v sdto.Details) (sdto.Details, error) {
	v.IndexNo = strings.TrimSpace(v.IndexNo)
	v.FullName = strings.TrimSpace(v.FullName)
	v.Faculty = strings.TrimSpace(v.Faculty)
	v.ContactPhone = strings.TrimSpace(v.ContactPhone)
	if !validation.Text(v.IndexNo, 64) || !validation.Text(v.FullName, 200) || !validation.Text(v.Faculty, 120) || v.Year < 1 || v.Year > 10 || !validation.Phone(v.ContactPhone) || len(v.Guardians) < 1 || len(v.Guardians) > 10 {
		return sdto.Details{}, sdomain.ErrInvalid
	}
	v.Guardians = append([]sdomain.GuardianInput(nil), v.Guardians...)
	for n, g := range v.Guardians {
		g.Name = strings.TrimSpace(g.Name)
		g.Relationship = strings.TrimSpace(g.Relationship)
		g.ContactPhone = strings.TrimSpace(g.ContactPhone)
		if !validation.Text(g.Name, 200) || !validation.Text(g.Relationship, 80) || !validation.Phone(g.ContactPhone) {
			return sdto.Details{}, sdomain.ErrInvalid
		}
		v.Guardians[n] = g
	}
	return v, nil
}
