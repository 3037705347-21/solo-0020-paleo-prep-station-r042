package station

import (
	"context"

	"example.com/paleo-prep-station/internal/contracts"
	"example.com/paleo-prep-station/internal/domain"
	"example.com/paleo-prep-station/internal/repository"
)

func toStepInputs(inputs []contracts.MethodStepInput) []domain.MethodStep {
	steps := make([]domain.MethodStep, 0, len(inputs))
	for _, input := range inputs {
		steps = append(steps, domain.MethodStep{
			Name:        input.Name,
			Instruction: input.Instruction,
		})
	}
	return steps
}

func toBoundary(input contracts.BoundaryInput) domain.ExposureBoundary {
	return domain.ExposureBoundary{Min: input.Min, Max: input.Max}
}

func (s *Service) CreateMethod(
	ctx context.Context,
	request contracts.CreateMethodRequest,
) (domain.StandardMethod, error) {
	if err := ctx.Err(); err != nil {
		return domain.StandardMethod{}, err
	}

	var created domain.StandardMethod
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		method, err := domain.NewStandardMethod(
			s.nextID("method"),
			request.Code,
			request.Name,
			request.Description,
			toStepInputs(request.Steps),
			append([]string(nil), request.Materials...),
			toBoundary(request.ExposureBound),
			s.now(),
		)
		if err != nil {
			return err
		}
		if _, exists := m.FindMethodByCode(method.Code); exists {
			return contracts.Conflict("duplicate_method_code", "方法编码已存在，不能覆盖原方法")
		}
		m.PutMethod(method)
		created = method
		return nil
	})
	return created, err
}

func (s *Service) ListMethods(ctx context.Context) ([]domain.StandardMethod, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var values []domain.StandardMethod
	err := s.repo.Read(func(m *repository.Mutation) error {
		values = m.ListMethods()
		return nil
	})
	return values, err
}

func (s *Service) GetMethod(ctx context.Context, methodID string) (domain.StandardMethod, error) {
	if err := ctx.Err(); err != nil {
		return domain.StandardMethod{}, err
	}
	var value domain.StandardMethod
	err := s.repo.Read(func(m *repository.Mutation) error {
		found, ok := m.GetMethod(methodID)
		if !ok {
			return notFoundMethod(methodID)
		}
		value = found
		return nil
	})
	return value, err
}

func (s *Service) CreateMethodDraft(
	ctx context.Context,
	methodID string,
	request contracts.NewMethodVersionRequest,
) (domain.MethodVersion, error) {
	if err := ctx.Err(); err != nil {
		return domain.MethodVersion{}, err
	}

	var created domain.MethodVersion
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		method, ok := m.GetMethod(methodID)
		if !ok {
			return notFoundMethod(methodID)
		}
		version, err := method.AddDraft(
			toStepInputs(request.Steps),
			append([]string(nil), request.Materials...),
			toBoundary(request.ExposureBound),
			s.now(),
		)
		if err != nil {
			return err
		}
		m.PutMethod(method)
		created = version
		return nil
	})
	return created, err
}

func (s *Service) UpdateMethodDraft(
	ctx context.Context,
	methodID string,
	versionNumber int,
	request contracts.NewMethodVersionRequest,
) (domain.MethodVersion, error) {
	if err := ctx.Err(); err != nil {
		return domain.MethodVersion{}, err
	}

	var updated domain.MethodVersion
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		method, ok := m.GetMethod(methodID)
		if !ok {
			return notFoundMethod(methodID)
		}
		if err := method.UpdateDraft(
			versionNumber,
			toStepInputs(request.Steps),
			append([]string(nil), request.Materials...),
			toBoundary(request.ExposureBound),
			s.now(),
		); err != nil {
			return err
		}
		version, ok := method.Version(versionNumber)
		if !ok {
			return contracts.NotFound("method_version_not_found", "未找到方法版本")
		}
		m.PutMethod(method)
		updated = version
		return nil
	})
	return updated, err
}

func (s *Service) PublishMethodVersion(
	ctx context.Context,
	methodID string,
	versionNumber int,
	request contracts.PublishMethodRequest,
) (domain.MethodVersion, error) {
	if err := ctx.Err(); err != nil {
		return domain.MethodVersion{}, err
	}

	var published domain.MethodVersion
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		method, ok := m.GetMethod(methodID)
		if !ok {
			return notFoundMethod(methodID)
		}
		version, err := method.Publish(versionNumber, request.PublishedBy, s.now())
		if err != nil {
			return err
		}
		m.PutMethod(method)
		published = version
		return nil
	})
	return published, err
}

func (s *Service) RetireMethodVersion(
	ctx context.Context,
	methodID string,
	versionNumber int,
	request contracts.RetireMethodRequest,
) (domain.MethodVersion, error) {
	if err := ctx.Err(); err != nil {
		return domain.MethodVersion{}, err
	}

	var retired domain.MethodVersion
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		method, ok := m.GetMethod(methodID)
		if !ok {
			return notFoundMethod(methodID)
		}
		if err := method.Retire(versionNumber, request.RetiredBy, s.now()); err != nil {
			return err
		}
		version, ok := method.Version(versionNumber)
		if !ok {
			return contracts.NotFound("method_version_not_found", "未找到方法版本")
		}
		m.PutMethod(method)
		retired = version
		return nil
	})
	return retired, err
}
