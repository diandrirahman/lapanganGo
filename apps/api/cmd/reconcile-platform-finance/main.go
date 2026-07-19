package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lapangango-api/internal/platformfinance"
)

type CLIOutput struct {
	Version string     `json:"version"`
	Report  *CLIReport `json:"report,omitempty"`
}

type CLIPeriod struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type CLIReport struct {
	Period   CLIPeriod          `json:"period"`
	Timezone string             `json:"timezone"`
	AsOf     time.Time          `json:"as_of"`
	Status   string             `json:"status"`
	Clean    bool               `json:"clean"`
	Checks   []CLICheckResult   `json:"checks"`
}

type CLICheckResult struct {
	Code             string         `json:"code"`
	Status           string         `json:"status"`
	DifferenceCount  int64          `json:"difference_count"`
	DifferenceRupiah int64          `json:"difference_rupiah"`
	Exceptions       []CLIException `json:"exceptions,omitempty"`
}

type CLIException struct {
	Metric           string `json:"metric"`
	BucketDate       string `json:"bucket_date"`
	ExpectedCount    int64  `json:"expected_count"`
	ActualCount      int64  `json:"actual_count"`
	DifferenceCount  int64  `json:"difference_count"`
	ExpectedRupiah   int64  `json:"expected_rupiah"`
	ActualRupiah     int64  `json:"actual_rupiah"`
	DifferenceRupiah int64  `json:"difference_rupiah"`
	// Note: Reason is intentionally omitted for sanitation
}

type runnerOperations struct {
	openPool     func(ctx context.Context, url string) (*pgxpool.Pool, error)
	closePool    func(pool *pgxpool.Pool)
	pingPool     func(ctx context.Context, pool *pgxpool.Pool) error
	buildService func(pool *pgxpool.Pool) platformfinance.ReconciliationService
}

func defaultRunnerOperations() runnerOperations {
	return runnerOperations{
		openPool: func(ctx context.Context, url string) (*pgxpool.Pool, error) {
			config, err := pgxpool.ParseConfig(url)
			if err != nil {
				return nil, err
			}
			config.ConnConfig.RuntimeParams["application_name"] = "lapanggo-reconcile-cli"
			return pgxpool.NewWithConfig(ctx, config)
		},
		closePool: func(pool *pgxpool.Pool) {
			if pool != nil {
				pool.Close()
			}
		},
		pingPool: func(ctx context.Context, pool *pgxpool.Pool) error {
			if pool != nil {
				return pool.Ping(ctx)
			}
			return nil
		},
		buildService: func(pool *pgxpool.Pool) platformfinance.ReconciliationService {
			provider := platformfinance.NewProductionReconciliationActualMetricsProvider()
			repo := platformfinance.NewReconciliationRepository(pool, provider)
			return platformfinance.NewReconciliationService(repo)
		},
	}
}

func main() {
	os.Exit(run(
		os.Args[1:],
		os.Getenv,
		os.Stdout,
		os.Stderr,
		defaultRunnerOperations(),
	))
}

func run(args []string, getenv func(string) string, stdout io.Writer, stderr io.Writer, ops runnerOperations) int {
	flagSet := flag.NewFlagSet("reconcile-platform-finance", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard) // Disable default error printing to stdout/stderr

	startDate := flagSet.String("start-date", "", "Start date (YYYY-MM-DD)")
	endDate := flagSet.String("end-date", "", "End date (YYYY-MM-DD)")

	if err := flagSet.Parse(args); err != nil {
		fmt.Fprintln(stderr, "invalid_arguments")
		return 1
	}

	if flagSet.NArg() != 0 {
		fmt.Fprintln(stderr, "invalid_arguments")
		return 1
	}

	if *startDate == "" || *endDate == "" {
		fmt.Fprintln(stderr, "invalid_arguments")
		return 1
	}

	if _, _, err := platformfinance.ParseAndValidateDates(*startDate, *endDate); err != nil {
		fmt.Fprintln(stderr, "invalid_arguments")
		return 1
	}

	dbURL := getenv("RECONCILIATION_DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(stderr, "setup_failed")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := ops.openPool(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(stderr, "setup_failed")
		return 1
	}
	defer ops.closePool(pool)

	if err := ops.pingPool(ctx, pool); err != nil {
		fmt.Fprintln(stderr, "setup_failed")
		return 1
	}

	service := ops.buildService(pool)
	query := platformfinance.ReconciliationQuery{
		StartDate: *startDate,
		EndDate:   *endDate,
	}

	report, err := service.Reconcile(ctx, query)
	if err != nil || report == nil {
		fmt.Fprintln(stderr, "reconciliation_failed")
		return 1
	}

	output := CLIOutput{
		Version: "1",
		Report:  mapReport(report),
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintln(stderr, "serialization_failed")
		return 1
	}

	if !report.Clean {
		return 1
	}

	return 0
}

func mapReport(r *platformfinance.ReconciliationReport) *CLIReport {
	if r == nil {
		return nil
	}
	out := &CLIReport{
		Period: CLIPeriod{
			StartDate: r.Period.StartDate,
			EndDate:   r.Period.EndDate,
		},
		Timezone: r.Timezone,
		AsOf:     r.AsOf.UTC(), // enforce UTC
		Status:   string(r.Status),
		Clean:    r.Clean,
		Checks:   make([]CLICheckResult, 0, len(r.Checks)),
	}

	for _, c := range r.Checks {
		check := CLICheckResult{
			Code:             c.Code,
			Status:           string(c.Status),
			DifferenceCount:  c.DifferenceCount,
			DifferenceRupiah: c.DifferenceRupiah,
			Exceptions:       make([]CLIException, 0, len(c.Exceptions)),
		}
		for _, e := range c.Exceptions {
			check.Exceptions = append(check.Exceptions, CLIException{
				Metric:           e.Metric,
				BucketDate:       e.BucketDate,
				ExpectedCount:    e.ExpectedCount,
				ActualCount:      e.ActualCount,
				DifferenceCount:  e.DifferenceCount,
				ExpectedRupiah:   e.ExpectedRupiah,
				ActualRupiah:     e.ActualRupiah,
				DifferenceRupiah: e.DifferenceRupiah,
			})
		}

		// Sort exceptions
		sort.SliceStable(check.Exceptions, func(i, j int) bool {
			e1, e2 := check.Exceptions[i], check.Exceptions[j]
			if e1.BucketDate != e2.BucketDate {
				return e1.BucketDate < e2.BucketDate
			}
			if e1.Metric != e2.Metric {
				return e1.Metric < e2.Metric
			}
			if e1.DifferenceCount != e2.DifferenceCount {
				return e1.DifferenceCount < e2.DifferenceCount
			}
			return e1.DifferenceRupiah < e2.DifferenceRupiah
		})

		out.Checks = append(out.Checks, check)
	}

	// Sort checks
	sort.SliceStable(out.Checks, func(i, j int) bool {
		return out.Checks[i].Code < out.Checks[j].Code
	})

	return out
}
