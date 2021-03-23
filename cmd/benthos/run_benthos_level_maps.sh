#!/bin/bash

# ensure the necessary context has been created on the
# n3 server
curl -s  -X POST http://localhost:1323/admin/newdemocontext -d userName=nsipOtfLevel -d contextName=levellingMaps


# now run the workflow
# 
benthos lint levelMaps.yaml
clear && benthos --chilled -c levelMaps.yaml
