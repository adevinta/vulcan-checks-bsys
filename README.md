# Vulcan Checks Build System

Set of scripts in go used to automate building and publishing Vulcan checks

# How to build and publish a single image to DEV environment

Bellow there is a snippet showing how to build the `vulcan-nessus` check and publish it to `DEV`.

```
git clone https://github.com/adevinta/vulcan-checks
cd vulcan-checks/

CGO_ENABLED=0 FORCE_BUILD="vulcan-nessus" TRAVIS_BRANCH="nessus" ../vulcan-checks-bsys/cmd/vulcan-detect-images/vulcan-detect-images cmd images_to_build

CGO_ENABLED=0 ../vulcan-checks-bsys/cmd/vulcan-build-images/vulcan-build-images -i ./images_to_build
```
