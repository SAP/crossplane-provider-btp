/*
This example demonstrates a CLI implementation with a subcommand using
the export CLI framework. When the subcommand is invoked, the CLI
widgets are demonstrated.

Try running the example with the following command

   go run ./main.go widget --help

As you can see, there is a 'select' command line flag that can be used
to select the widget to test.

The possible values are

   - text
   - sensitive
   - multi

If you don't specify the 'select' parameter, the CLI example will ask
for the widgets to test:

   go run ./main.go widget

You can also specify the select parameter using CLI flags:

   go run ./main.go widget --select text

Or using the shorthand version:

   go run ./main.go widget -s multi

You can specify multiple select values:

   go run ./main.go widget -s multi -s sensitive

The 'select' parameter can also be set using the 'SELECT' environment
variable.

   SELECT="multi" go run ./main.go widget

Multiple selections shall be separated by a space:

   SELECT="text sensitive" go run ./main.go widget

Finally, the `select` parameter can also be set using a
configuratin file. If `config.yaml` file contains the following:

   select:
     - text
     - multi

the select paramet is read from the config file with this command:

   go run ./main.go widget --config config.yaml

This example also demonstrates the verbose logging capabilities using
the '-v' flag.
*/
package main
