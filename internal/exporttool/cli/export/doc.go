/*
Package export defines the export subcommand. The subcommand has
two predefined configuration parameters: 'kind' and 'output'.

The business logic of the export command is set using theh
[SetCommand] function.

The business logic is defined with the following function type:

	func(ctx context.Context, eventHandler EventHandler) error

The business logic shall generate Kubernetes resource object values of
type [resource.Object]. These objects must be reported using the

	Resource(resource.Object)

method. The reported resources printed on the console on stored in the
output file.

In case of non-fatal errors, the errors can be reported using the

	Warn(error)

method. The reported errors are printed on the console to STDERR.

The business logic can signal that the processing is stopped using the

	Stop()

method.
*/
package export
