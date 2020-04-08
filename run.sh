#!/bin/sh

# Apply env variables
cat config.toml | envsubst > run.toml

/app/vulcan-build-images -c run.toml -p $PERSISTENCE_URL
