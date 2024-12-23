package submission

import (
	"context"
	"errors"
	"github.com/upassed/upassed-answer-service/internal/async"
	"github.com/upassed/upassed-answer-service/internal/handling"
	"github.com/upassed/upassed-answer-service/internal/logging"
	"github.com/upassed/upassed-answer-service/internal/middleware/common/auth"
	business "github.com/upassed/upassed-answer-service/internal/service/model"
	"github.com/upassed/upassed-answer-service/internal/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"log/slog"
)

var (
	ErrSubmissionCreateDeadlineExceeded = errors.New("")
)

func (service *serviceImpl) Create(ctx context.Context, submission *business.Submission) (*business.SubmissionCreateResponse, error) {
	studentUsername := ctx.Value(auth.UsernameKey).(string)

	spanContext, span := otel.Tracer(service.cfg.Tracing.SubmissionTracerName).Start(ctx, "submissionService#Create")
	span.SetAttributes(
		attribute.String("formID", submission.FormID.String()),
		attribute.String("questionID", submission.QuestionID.String()),
		attribute.String(auth.UsernameKey, studentUsername),
	)
	defer span.End()

	log := logging.Wrap(service.log,
		logging.WithOp(service.Create),
		logging.WithCtx(ctx),
		logging.WithAny("formID", submission.FormID.String()),
		logging.WithAny("questionID", submission.QuestionID.String()),
	)

	log.Info("started creating submission")
	timeout := service.cfg.GetEndpointExecutionTimeout()

	submissionCreateResponse, err := async.ExecuteWithTimeout(spanContext, timeout, func(ctx context.Context) (*business.SubmissionCreateResponse, error) {
		log.Info("saving submission data to the database")
		domainSubmissions := ConvertToDomainSubmissions(submission)

		if err := service.repository.Save(ctx, domainSubmissions); err != nil {
			log.Error("unable to save submission data to the database", logging.Error(err))
			tracing.SetSpanError(span, err)
			return nil, err
		}

		return ConvertToSubmissionCreateResponse(domainSubmissions), nil
	})

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Error("submission creating deadline exceeded")
			tracing.SetSpanError(span, err)
			return nil, handling.Wrap(ErrSubmissionCreateDeadlineExceeded, handling.WithCode(codes.DeadlineExceeded))
		}

		log.Error("error while creating submission", logging.Error(err))
		tracing.SetSpanError(span, err)
		return nil, handling.Process(err)
	}

	log.Info("submission successfully created", slog.Any("createdSubmissionIDs", submissionCreateResponse.CreatedSubmissionIDs))
	return submissionCreateResponse, nil
}