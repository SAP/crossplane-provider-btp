#!/usr/bin/env bash
cd ..
# expects a list of paths relative to internal/controller and will remove those controllers and their references as well as the crds belonging to that one
for folder in "$@"; do
  controllerFolder="internal/controller/$folder"
  # extract package name as used in zz_setup.go
  package=$(grep "$controllerFolder" internal/controller/zz_setup.go | grep -Eo '^[^ ]+' | sed -e 's/^[ \t]*//')
  # remove all usages of that reference from file
  awk "!/$package/" internal/controller/zz_setup.go > tmpfile && mv tmpfile internal/controller/zz_setup.go
  # remove controller folder itself
  rm -rf "$controllerFolder"
  # remove crd from package/crds folder
  pluralname=$(basename "$folder")s
  rm package/crds/*$pluralname*
  # remove generated examples
  rm examples-generated/*/*/$(basename "$folder").yaml 
done





