/*
Package configparam defines the configuration parameters that a CLI
tool can use.

The values of the configuration parameters can be set via CLI
flags, environment variables. A CLI flag shall be attached to a
[cobra.Command] using the AttachToCommand method.

The BindConfiguration method binds the configured environment
variable name. It shall be invoked only for the command that is
executed. The recommendation is to invoke it in the PreRun phase of
the [cobra.Command].

The configured value of the parameter can be read using the Value
and the ValueOrAsk methods.

Value returns the configured value if set. If the parameter value
is not set, it returns the default value.

ValueOrAsk also returns the configured value if set. If not set, it
asks the user for the value interactively.
*/
package configparam
