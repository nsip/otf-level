#!/bin/bash


rm -r ./msgs/level
mkdir ./msgs/level

benthos lint levelData.yaml

clear && benthos -c levelData.yaml
