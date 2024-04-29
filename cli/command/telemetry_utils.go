package command

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/cli/cli/version"
	"github.com/moby/term"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// BaseCommandAttributes returns an attribute.Set containing attributes to attach to metrics/traces
func BaseCommandAttributes(cmd *cobra.Command, streams Streams) []attribute.KeyValue {
	return append([]attribute.KeyValue{
		attribute.String("command.name", getCommandName(cmd)),
	}, stdioAttributes(streams)...)
}

// InstrumentCobraCommands wraps all cobra commands' RunE funcs to set a command duration metric using otel.
//
// Note: this should be the last func to wrap/modify the PersistentRunE/RunE funcs before command execution.
//
// can also be used for spans!
func (cli *DockerCli) InstrumentCobraCommands(cmd *cobra.Command) {
	// If PersistentPreRunE is nil, make it execute PersistentPreRun and return nil by default
	ogPersistentPreRunE := cmd.PersistentPreRunE
	if ogPersistentPreRunE == nil {
		ogPersistentPreRun := cmd.PersistentPreRun
		//nolint:unparam // necessary because error will always be nil here
		ogPersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			ogPersistentPreRun(cmd, args)
			return nil
		}
		cmd.PersistentPreRun = nil
	}

	// wrap RunE in PersistentPreRunE so that this operation gets executed on all children commands
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// If RunE is nil, make it execute Run and return nil by default
		ogRunE := cmd.RunE
		if ogRunE == nil {
			ogRun := cmd.Run
			//nolint:unparam // necessary because error will always be nil here
			ogRunE = func(cmd *cobra.Command, args []string) error {
				ogRun(cmd, args)
				return nil
			}
			cmd.Run = nil
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			// start the timer as the first step of every cobra command
			baseAttrs := BaseCommandAttributes(cmd, cli)
			stopCobraCmdTimer := startCobraCommandTimer(cmd, GetDefaultMeter(), baseAttrs)

			cmdErr := ogRunE(cmd, args)
			stopCobraCmdTimer(cmdErr)
			return cmdErr
		}

		return ogPersistentPreRunE(cmd, args)
	}
}

func startCobraCommandTimer(cmd *cobra.Command, meter metric.Meter, attrs []attribute.KeyValue) func(err error) {
	ctx := cmd.Context()
	durationCounter, _ := meter.Float64Counter(
		"command.time",
		metric.WithDescription("Measures the duration of the cobra command"),
		metric.WithUnit("ms"),
	)
	start := time.Now()

	return func(err error) {
		duration := float64(time.Since(start)) / float64(time.Millisecond)
		cmdStatusAttrs := attributesFromCommandError(err)
		durationCounter.Add(ctx, duration,
			metric.WithAttributes(attrs...),
			metric.WithAttributes(cmdStatusAttrs...),
		)
	}
}

// BasePluginCommandAttributes returns a slice of attribute.KeyValue to attach to metrics/traces
func BasePluginCommandAttributes(plugincmd *pluginCmd, streams Streams) []attribute.KeyValue {
	return append([]attribute.KeyValue{
		attribute.String("plugin.command.string", plugincmd.String()),
		attribute.String("plugin.command.path", plugincmd.Path),
		attribute.String("plugin.command.dir", plugincmd.Dir),
		attribute.StringSlice("plugin.command.string", plugincmd.Args),
	}, stdioAttributes(streams)...)
}

// pluginCmd is used to wrap an exec.Cmd in order to instrument the command request with otel
// with a custom Run() implementation
type pluginCmd struct {
	*exec.Cmd

	ctx       context.Context
	baseAttrs []attribute.KeyValue
}

func (c *pluginCmd) setContext(ctx context.Context) {
	c.ctx = ctx
}

func (c *pluginCmd) setBaseAttrs(attrs []attribute.KeyValue) {
	c.baseAttrs = attrs
}

func (c *pluginCmd) Run() error {
	stopPluginCommandTimer := StartPluginCommandTimer(c.ctx, c.baseAttrs)
	err := c.Cmd.Run()
	stopPluginCommandTimer(err)
	return err
}

func InstrumentPluginCommand(ctx context.Context, plugincmd *pluginCmd, cli Cli) *pluginCmd {
	baseAttrs := BasePluginCommandAttributes(plugincmd, cli)

	newCmd := &pluginCmd{Cmd: exec.Command(plugincmd.Path, plugincmd.Args...)}
	newCmd.setContext(ctx)
	newCmd.setBaseAttrs(baseAttrs)

	return newCmd
}

func StartPluginCommandTimer(ctx context.Context, attrs []attribute.KeyValue) func(err error) {
	durationCounter, _ := GetDefaultMeter().Float64Counter(
		"plugin.command.time",
		metric.WithDescription("Measures the duration of the plugin execution"),
		metric.WithUnit("ms"),
	)
	start := time.Now()

	return func(err error) {
		duration := float64(time.Since(start)) / float64(time.Millisecond)
		pluginStatusAttrs := attributesFromPluginError(err)
		durationCounter.Add(ctx, duration,
			metric.WithAttributes(attrs...),
			metric.WithAttributes(pluginStatusAttrs...),
		)
	}
}

func stdioAttributes(streams Streams) []attribute.KeyValue {
	// we don't wrap stderr, but we do wrap in/out
	_, stderrTty := term.GetFdInfo(streams.Err())
	return []attribute.KeyValue{
		attribute.Bool("command.stdin.isatty", streams.In().IsTerminal()),
		attribute.Bool("command.stdout.isatty", streams.Out().IsTerminal()),
		attribute.Bool("command.stderr.isatty", stderrTty),
	}
}

// Used to create attributes from an error.
// The error is expected to be returned from the execution of a cobra command
func attributesFromCommandError(err error) []attribute.KeyValue {
	attrs := []attribute.KeyValue{}
	exitCode := 0
	if err != nil {
		exitCode = 1
		if stderr, ok := err.(statusError); ok {
			// StatusError should only be used for errors, and all errors should
			// have a non-zero exit status, so only set this here if this value isn't 0
			if stderr.StatusCode != 0 {
				exitCode = stderr.StatusCode
			}
		}
		attrs = append(attrs, attribute.String("command.error.type", otelErrorType(err)))
	}
	attrs = append(attrs, attribute.Int("command.status.code", exitCode))

	return attrs
}

// Used to create attributes from an error.
// The error is expected to be returned from the execution of a plugin
func attributesFromPluginError(err error) []attribute.KeyValue {
	attrs := []attribute.KeyValue{}
	exitCode := 0
	if err != nil {
		exitCode = 1
		if stderr, ok := err.(statusError); ok {
			// StatusError should only be used for errors, and all errors should
			// have a non-zero exit status, so only set this here if this value isn't 0
			if stderr.StatusCode != 0 {
				exitCode = stderr.StatusCode
			}
		}
		attrs = append(attrs, attribute.String("plugin.error.type", otelErrorType(err)))
	}
	attrs = append(attrs, attribute.Int("plugin.status.code", exitCode))

	return attrs
}

// otelErrorType returns an attribute for the error type based on the error category.
func otelErrorType(err error) string {
	name := "generic"
	if errors.Is(err, context.Canceled) {
		name = "canceled"
	}
	return name
}

// statusError reports an unsuccessful exit by a command.
type statusError struct {
	Status     string
	StatusCode int
}

func (e statusError) Error() string {
	return fmt.Sprintf("Status: %s, Code: %d", e.Status, e.StatusCode)
}

// getCommandName gets the cobra command name in the format
// `... parentCommandName commandName` by traversing it's parent commands recursively.
// until the root command is reached.
//
// Note: The root command's name is excluded. If cmd is the root cmd, return ""
func getCommandName(cmd *cobra.Command) string {
	fullCmdName := getFullCommandName(cmd)
	i := strings.Index(fullCmdName, " ")
	if i == -1 {
		return ""
	}
	return fullCmdName[i+1:]
}

// getFullCommandName gets the full cobra command name in the format
// `... parentCommandName commandName` by traversing it's parent commands recursively
// until the root command is reached.
func getFullCommandName(cmd *cobra.Command) string {
	if cmd.HasParent() {
		return fmt.Sprintf("%s %s", getFullCommandName(cmd.Parent()), cmd.Name())
	}
	return cmd.Name()
}

// getDefaultMeter gets the default metric.Meter for the application
// using the given metric.MeterProvider
func getDefaultMeter(mp metric.MeterProvider) metric.Meter {
	return mp.Meter(
		"github.com/docker/cli",
		metric.WithInstrumentationVersion(version.Version),
	)
}

// GetDefaultMeter gets the default metric.Meter for the application
// using the global metric.MeterProvider
func GetDefaultMeter() metric.Meter {
	return otel.Meter(
		"github.com/docker/cli",
		metric.WithInstrumentationVersion(version.Version),
	)
}

// GetDefaultTracer gets the default trace.Tracer for the application
// using the global trace.TraceProvider
func GetDefaultTracer() trace.Tracer {
	return otel.Tracer(
		"github.com/docker/cli",
		trace.WithInstrumentationVersion(version.Version),
	)
}
