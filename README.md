# Vulcan Checks Build System

Set of scripts in go used to automate building and publishing Vulcan checks

## How to build and publish a single image to DEV environment

Bellow there is a snippet showing how to build the `vulcan-nessus` check and publish it to `DEV`.

```sh
git clone https://github.com/adevinta/vulcan-checks
cd vulcan-checks/

CGO_ENABLED=0 FORCE_BUILD="vulcan-nessus" TRAVIS_BRANCH="nessus" ../vulcan-checks-bsys/cmd/vulcan-detect-images/vulcan-detect-images cmd images_to_build

CGO_ENABLED=0 ../vulcan-checks-bsys/cmd/vulcan-build-images/vulcan-build-images -i ./images_to_build
```

## How to run a check locally and generate a report with its output

You will also have to install the [security-overview](https://github.com/adevinta/security-overview) command line
tool and have a [local.toml](https://github.com/adevinta/vulcan-checks/blob/master/cmd/vulcan-exposed-http/local.toml.example) file in the check's directory.

Example:

```sh
git clone https://github.com/adevinta/vulcan-checks
cd vulcan-checks/
vulcan-build-images -r cmd/vulcan-http-headers -o ./check_report.json
vulcan-security-overview -config security-overview.toml -check check_report.json
```
