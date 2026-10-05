package station

import (
	"context"
	"strings"

	"example.com/paleo-prep-station/internal/contracts"
	"example.com/paleo-prep-station/internal/domain"
	"example.com/paleo-prep-station/internal/repository"
)

func (s *Service) CreateMethod(
	ctx context.Context,
	request contracts.CreateMethodRequest,
) (domain.MethodStandard, error) {
	if err := ctx.Err(); err != nil {
		return domain.MethodStandard{}, err
	}

	var created domain.MethodStandard
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		value, err := domain.NewMethodStandard(
			s.nextID("method"),
			request.Code,
			request.Name,
			request.Description,
			methodDraftFromRequest(request.Steps, request.AllowedAdhesives, request.TargetMin, request.TargetMax),
			s.now(),
		)
		if err != nil {
			return err
		}
		if _, exists := m.FindMethodByCode(value.Code); exists {
			return contracts.Conflict("duplicate_method_code", "方法编码已存在，不能覆盖原标准方法")
		}
		m.PutMethod(value)
		created = value
		return nil
	})
	return created, err
}

func (s *Service) ListMethods(ctx context.Context) ([]domain.MethodStandard, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var values []domain.MethodStandard
	err := s.repo.Read(func(m *repository.Mutation) error {
		values = m.ListMethods()
		return nil
	})
	return values, err
}

func (s *Service) GetMethod(ctx context.Context, code string) (domain.MethodStandard, error) {
	if err := ctx.Err(); err != nil {
		return domain.MethodStandard{}, err
	}
	var value domain.MethodStandard
	err := s.repo.Read(func(m *repository.Mutation) error {
		found, ok := m.FindMethodByCode(normalizeMethodCode(code))
		if !ok {
			return notFoundMethod(code)
		}
		value = found
		return nil
	})
	return value, err
}

func (s *Service) CreateMethodDraft(
	ctx context.Context,
	code string,
	request contracts.MethodDraftRequest,
) (domain.MethodStandard, error) {
	if err := ctx.Err(); err != nil {
		return domain.MethodStandard{}, err
	}

	var updated domain.MethodStandard
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		method, ok := m.FindMethodByCode(normalizeMethodCode(code))
		if !ok {
			return notFoundMethod(code)
		}
		if _, err := method.AddDraft(
			methodDraftFromRequest(request.Steps, request.AllowedAdhesives, request.TargetMin, request.TargetMax),
			s.now(),
		); err != nil {
			return err
		}
		m.PutMethod(method)
		updated = method
		return nil
	})
	return updated, err
}

func (s *Service) PublishMethodVersion(
	ctx context.Context,
	code string,
	version int,
	request contracts.PublishMethodRequest,
) (domain.MethodStandard, error) {
	if err := ctx.Err(); err != nil {
		return domain.MethodStandard{}, err
	}

	var updated domain.MethodStandard
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		method, ok := m.FindMethodByCode(normalizeMethodCode(code))
		if !ok {
			return notFoundMethod(code)
		}
		if err := method.Publish(version, request.PublishedBy, s.now()); err != nil {
			return err
		}
		m.PutMethod(method)
		updated = method
		return nil
	})
	return updated, err
}

func (s *Service) RetireMethodVersion(
	ctx context.Context,
	code string,
	version int,
	request contracts.RetireMethodRequest,
) (domain.MethodStandard, error) {
	if err := ctx.Err(); err != nil {
		return domain.MethodStandard{}, err
	}

	var updated domain.MethodStandard
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		method, ok := m.FindMethodByCode(normalizeMethodCode(code))
		if !ok {
			return notFoundMethod(code)
		}
		if err := method.Retire(version, request.RetiredBy, s.now()); err != nil {
			return err
		}
		m.PutMethod(method)
		updated = method
		return nil
	})
	return updated, err
}

func methodDraftFromRequest(
	steps []string,
	adhesives []string,
	targetMin int,
	targetMax int,
) domain.MethodDraftInput {
	return domain.MethodDraftInput{
		Steps:            steps,
		AllowedAdhesives: adhesives,
		TargetMin:        targetMin,
		TargetMax:        targetMax,
	}
}

func normalizeMethodCode(code string) string {
	normalized, err := domain.NormalizeMethodCode(code)
	if err != nil {
		return strings.ToUpper(strings.TrimSpace(code))
	}
	return normalized
}
