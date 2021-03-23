#!/bin/bash

benthos lint levelData.yaml

clear && benthos --chilled -c levelData.yaml
