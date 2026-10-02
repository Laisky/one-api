#!/bin/sh

version=$(date +%s)
pwd

while IFS= read -r theme; do
    echo "Building theme: $theme"
    rm -r build/$theme
    cd "$theme"
    yarn install
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$version yarn run build
    cd ..
done < THEMES
