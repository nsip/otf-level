#!/bin/bash

benthos lint levelData.yaml

clear && benthos -c levelData.yaml
