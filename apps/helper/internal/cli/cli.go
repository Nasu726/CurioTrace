package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/observation"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/platformpath"
)

const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitUnavailable = 3
)

const (
	ReasonBootstrapUnavailable = "PRODUCTION_BOOTSTRAP_UNAVAILABLE"
	ReasonInspectUnavailable   = "SESSION_INSPECTION_UNAVAILABLE"
	ReasonSessionReadFailed    = "SESSION_READ_FAILED"
)

type SessionReader interface {
	ListSession(ctx context.Context, sessionID string) ([]observation.ValidatedEvent, error)
}

type Readiness struct {
	RecordingAvailable  bool   `json:"recording_available"`
	RecordingReason     string `json:"recording_reason,omitempty"`
	InspectionAvailable bool   `json:"inspection_available"`
	InspectionReason    string `json:"inspection_reason,omitempty"`
}

type Dependencies struct {
	GOOS         string
	ResolvePaths func() (platformpath.Paths, error)
	Reader       SessionReader
	Readiness    func(context.Context) Readiness
}

type StatusReport struct {
	Platform     string    `json:"platform"`
	DataRoot     string    `json:"data_root"`
	Observations string    `json:"observations_root"`
	Authority    string    `json:"authority_root"`
	Readiness    Readiness `json:"readiness"`
}

func DefaultDependencies() Dependencies {
	return Dependencies{
		GOOS:         runtime.GOOS,
		ResolvePaths: platformpath.Current,
		Readiness: func(context.Context) Readiness {
			// The native OS secret-store adapter and production bootstrap are
			// intentionally not wired yet. Do not silently substitute a
			// plaintext/testing key path merely to make the CLI look ready.
			return Readiness{
				RecordingAvailable:  false,
				RecordingReason:     ReasonBootstrapUnavailable,
				InspectionAvailable: false,
				InspectionReason:    ReasonInspectUnavailable,
			}
		},
	}
}

func Run(ctx context.Context, args []string, out, errOut io.Writer, deps Dependencies) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	if len(args) == 0 {
		writeUsage(errOut)
		return ExitUsage
	}

	switch args[0] {
	case "help", "-h", "--help":
		writeUsage(out)
		return ExitOK
	case "status":
		return runStatus(ctx, args[1:], out, errOut, deps)
	case "inspect":
		return runInspect(ctx, args[1:], out, errOut, deps)
	default:
		fmt.Fprintf(errOut, "curiotrace: unknown command %q\n", args[0])
		writeUsage(errOut)
		return ExitUsage
	}
}

func runStatus(ctx context.Context, args []string, out, errOut io.Writer, deps Dependencies) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(errOut)
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(errOut, "curiotrace status: unexpected positional arguments")
		return ExitUsage
	}
	if deps.ResolvePaths == nil {
		fmt.Fprintln(errOut, "curiotrace status: platform path resolver is unavailable")
		return ExitFailure
	}
	paths, err := deps.ResolvePaths()
	if err != nil {
		fmt.Fprintf(errOut, "curiotrace status: resolve platform paths: %v\n", err)
		return ExitFailure
	}
	readiness := Readiness{
		RecordingAvailable:  false,
		RecordingReason:     ReasonBootstrapUnavailable,
		InspectionAvailable: deps.Reader != nil,
	}
	if deps.Readiness != nil {
		readiness = deps.Readiness(ctx)
	}
	if deps.Reader != nil && readiness.InspectionReason == ReasonInspectUnavailable {
		readiness.InspectionAvailable = true
		readiness.InspectionReason = ""
	}
	report := StatusReport{
		Platform:     deps.GOOS,
		DataRoot:     paths.Root,
		Observations: paths.Observations,
		Authority:    paths.Authority,
		Readiness:    readiness,
	}
	if *jsonOutput {
		if err := writeJSON(out, report); err != nil {
			fmt.Fprintf(errOut, "curiotrace status: encode JSON: %v\n", err)
			return ExitFailure
		}
		return ExitOK
	}

	fmt.Fprintln(out, "CurioTrace status")
	fmt.Fprintf(out, "platform: %s\n", valueOrUnknown(report.Platform))
	fmt.Fprintf(out, "data root: %s\n", report.DataRoot)
	fmt.Fprintf(out, "observations: %s\n", report.Observations)
	fmt.Fprintf(out, "authority: %s\n", report.Authority)
	writeAvailability(out, "recording bootstrap", report.Readiness.RecordingAvailable, report.Readiness.RecordingReason)
	writeAvailability(out, "session inspection", report.Readiness.InspectionAvailable, report.Readiness.InspectionReason)
	return ExitOK
}

func runInspect(ctx context.Context, args []string, out, errOut io.Writer, deps Dependencies) int {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(errOut)
	sessionID := flags.String("session", "", "session id to inspect")
	jsonOutput := flags.Bool("json", false, "emit validated events as JSON")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 || !validSessionID(*sessionID) {
		fmt.Fprintln(errOut, "curiotrace inspect: --session must be a valid ses_<32 lowercase hex> id")
		return ExitUsage
	}
	if deps.Reader == nil {
		fmt.Fprintf(errOut, "curiotrace inspect: unavailable (%s); production encrypted-store bootstrap is not configured\n", ReasonInspectUnavailable)
		return ExitUnavailable
	}

	events, err := deps.Reader.ListSession(ctx, *sessionID)
	if err != nil {
		// Do not echo arbitrary lower-layer errors here. A future adapter error
		// must not become a side channel for observation/key material.
		fmt.Fprintf(errOut, "curiotrace inspect: failed (%s)\n", ReasonSessionReadFailed)
		return ExitFailure
	}
	if *jsonOutput {
		if err := writeJSON(out, events); err != nil {
			fmt.Fprintf(errOut, "curiotrace inspect: encode JSON: %v\n", err)
			return ExitFailure
		}
		return ExitOK
	}
	if err := writeHumanSession(out, *sessionID, events); err != nil {
		fmt.Fprintf(errOut, "curiotrace inspect: render session: %v\n", err)
		return ExitFailure
	}
	return ExitOK
}

func writeHumanSession(out io.Writer, sessionID string, events []observation.ValidatedEvent) error {
	fmt.Fprintf(out, "Session %s\n", sessionID)
	fmt.Fprintf(out, "events: %d\n", len(events))
	if len(events) == 0 {
		fmt.Fprintln(out, "(no durable observations)")
		return nil
	}

	for index, validated := range events {
		raw, err := json.Marshal(validated)
		if err != nil {
			return err
		}
		var event observation.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			return err
		}
		fmt.Fprintf(out, "\n%d. %s  %s\n", index+1, event.WallTime, event.EventType)
		if event.CaptureMode != "" {
			fmt.Fprintf(out, "   capture: %s\n", event.CaptureMode)
		}
		if event.Source != nil {
			if event.Source.Title != "" {
				fmt.Fprintf(out, "   title: %s\n", event.Source.Title)
			}
			if event.Source.URL != "" {
				fmt.Fprintf(out, "   url: %s\n", event.Source.URL)
			}
			if event.Source.CanonicalURL != "" && event.Source.CanonicalURL != event.Source.URL {
				fmt.Fprintf(out, "   canonical: %s\n", event.Source.CanonicalURL)
			}
		}
		if len(event.Payload) != 0 {
			payload, err := json.Marshal(event.Payload)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "   payload: %s\n", payload)
		}
	}
	return nil
}

func writeAvailability(out io.Writer, label string, available bool, reason string) {
	if available {
		fmt.Fprintf(out, "%s: available\n", label)
		return
	}
	fmt.Fprintf(out, "%s: unavailable", label)
	if reason != "" {
		fmt.Fprintf(out, " (%s)", reason)
	}
	fmt.Fprintln(out)
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  curiotrace status [--json]")
	fmt.Fprintln(out, "  curiotrace inspect --session ses_<32 lowercase hex> [--json]")
	fmt.Fprintln(out, "  curiotrace help")
}

func validSessionID(value string) bool {
	if len(value) != len("ses_")+32 || !strings.HasPrefix(value, "ses_") {
		return false
	}
	for _, ch := range value[len("ses_"):] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

