# otf-level
Web-service for consistent scaling of nnlp-oriented assessment results

# build

```
cd cmd
go build
```

# usage
otf-level is a web service that accepts simple requests for consistent scoring of assessment data against the National Learning Progressions common scale.

The otf-align API is simple:

```
> curl -v http://localhost:1327/level \ 
> -H 'Content-Type: application/json' \
  -d '{"levelMethod":"prescribed", \
        "levelProgLevel":"NNInF4", \
        "assessmentToken":"mastered", \
        "assessmentScore":250}
```

the two required parameters are:
- levelMethod: choice of mapped | prescribed (see full description below)
- levelProgLevel: the NNLP Progression Level for this query
the other two parameters can be used jointly or singly:
- assessmentScore: a numeric result of the assessment
- assessmentToken: a textual result - such as A-F grade or a judgement such as 'mastery'

the otf-level service will respond on success with the following data structure:
```
{
  "assessmentScore": 250,
  "assessmentToken": "mastered",
  "calculatedLevel": {
    "achievement": "mastered",
    "progressionLevel": "NNInF4",
    "scaledScore": 505
  },
  "levelMethod": "prescribed",
  "levelServiceID": "OsLV3uoKUPhwaN8sMgLBlZ",
  "levelServiceName": "yzAm5"
}
```

The response echoes the input parameters for completeness, and identifies the service instance that processed the request.
The result of the levelling is in the calculatedLevel block.

All configuration options can be set on the command-line using flags, via envronment variables, or by using a configuration file.
Configuration can use any or all of these methods in combination.
For example options such as the address and hostname of the classifier server might best be accessed from environment variables, whilst the service name of the otf-align instance might be supplied in a json configuration file.

Configuration flags are capitalised and prefixed with OTF_ALIGN_SRVC when supplied as environment variables; so flag --niasPort on the commnad-line becomes 

```
OTF_ALIGN_SRVC_NIASPORT=1323
```

when expressed as an environment variable and

```
{ "niasPort":1323 }
```

when set in a json configuration file.

These are the configuration options:

|Option name|Type|Required|Default|Description|
|---|---|---|---|---|
|config|string|no||configuration file name|
|name|string|yes|auto-generated (hashid)|name of this instance of the service|
|id|string|yes|auto-generated (nuid)|identifier for this service instance|  
|host|string|yes|localhost|host address to run this service on|
|port|int|yes|auto-generated|port to run the service on|
|niasHost|string|yes|localhost|host of n3w service|
|niasPort|int|yes|1323|port of the n3w service|
|niasToken|string|yes|a demo token|jwt token for accessing the n3w server|

# levelling methods
otf-level is a facade service which will invoke further services in order to determine the nationally scaled score of a particular assessment result or observation.

The levelling process begins by retrieving the score map for any given NNLP ProgressionLevel (suppplied as an input parameter). This details the available range of scores for the ProgressionLevel, as well as high-water-mark scores for partial and full achievement of a level.

Two styles of levelling are currently supported:
- mapped 
    -  alignment is resolved by mapping tokens from the original observation/assessment data to existing data structures that themselves are linked to the NNLP standard scale. For example, an assessment result may contain a numeric score or a letter-grade or some other textual judgement. The leveller uses a map of values from the provider system, or links to a known scale such as NAPLAN, which are resolved to numbers or ranges in the national scale.
- prescribed    
    - the provider assessment supplies an indicator token such as 'mastered' or 'achieved', but no supporting score information. The judgement is used to extract a suitable value from the score-map for the progression-level.

# pre-requisites
The otf-level service requires supporting services to be available:
- n3w, provides the lookup graphs for levelling
    + binary can be created from http://github.com/nsip/n3-web
- benthos, workflow engine installed and available
- nats-streaming-server, message broker installed and avialable


# benthos workflow

### note 
The easiest way to set up and run the whole PDM workflow is to get the
Docker images which will be available soon...
In the meantime, you can find a tmux script that starts all the services and dependencies here:
https://github.com/nsip/otf-testdata/tree/master/tmux/tmux_pdm.sh


As with otf-align the service is packaged with a benthos workflow which allows the testing of otf-align in context and interacting with the other progress data management services.

The levelling workflow assumes that the align workflow is already up and running, see the otf-align project for instructions on this. 
The otf-level reads data from a nats queue fed by the ouput of the otf-align service.

Prior to running the worklfow set an environment variable 
```
export PDM_ROOT=~/otfdata 
```
as a location for the workflow to write its data to

In order to add the otf-level service to the chain, first start the otf-level service
```
> ./otf-level --port 1327
```

then navigate to the benthos directory and run the data-processing workflow
```
otf-level/cmd/benthos> ./run_benthos_level_data.sh
```

which will connect to the existing chain of services and send data to the levelling service, and post the response messages to
```
${PDM_ROOT}/audit/level
```

A fully processed message example:
```
{
    "meta":
    {
        "alignMethod": "prescribed",
        "capability": "literacy",
        "inputFormat": "csv",
        "levelMethod": "prescribed",
        "messageID": "hdxntUGlS96oqdXMfsvG4M",
        "providerName": "SPA",
        "readTimestampUTC": "2020-07-16T12:42:43Z",
        "readerID": "hdxntUGlS96oqdXMfsvEFY",
        "readerName": "2vd71",
        "sourceFileName": "/Users/mattfarmer/otfdata/in/spa/sreams.prescribed.csv"
    },
    "original":
    {
        "1": "D",
        "2": "A",
        "3": 0,
        "4": "A",
        "5": "A",
        "6": "B",
        "7": "D",
        "8": "D",
        "9": 1,
        "Class": "4/5B",
        "FirstName": "Ethan",
        "LastName": "NGO",
        "StudentID": "NGO5789",
        "TestCode": "LWCrT5.1",
        "TestRound": 1,
        "TimeStamp": "2/26/20 10:51",
        "YearLevel": 4
    },
    "otf":
    {
        "align":
        {
            "alignmentServiceID": "2a4PFSRa5ESc0yztEBO1Vh",
            "alignmentServiceName": "vQ2bYl",
            "alignments": [
            {
                "element": "https://mracqa.azurewebsites.net/elements/learning-progressions/3.0/2020/06/2ef73b1b-493b-41d6-8d7a-342522d1c4e7",
                "generalCapability": "https://mracqa.azurewebsites.net/elements/learning-progressions/3.0/2020/06/5316eb98-cdee-4fba-bec4-074271e0954f",
                "heading": "https://mracqa.azurewebsites.net/elements/learning-progressions/3.0/2020/06/963086d8-4848-4cb0-a725-1a00925f4aaa",
                "indicator": "LWCrT5.1",
                "indicatorType": "https://mracqa.azurewebsites.net/elements/learning-progressions/3.0/2020/06/1f28879a-8165-4712-8c80-26be46d4cc8b",
                "progressionLevel": "LWCrT5",
                "subElement": "https://mracqa.azurewebsites.net/elements/learning-progressions/3.0/2020/06/de512a50-1c52-40d0-b9b9-27151e0cff48"
            }],
            "capability": "literacy",
            "method": "prescribed",
            "token": "LWCrT5.1"
        },
        "id":
        {
            "studentFamilyName": "NGO",
            "studentGivenName": "Ethan",
            "studentID": "NGO5789"
        },
        "level":
        {
            "assessmentToken": "D",
            "calculatedScore":
            {
                "achievement": "partially achieved",
                "progressionLevel": "LWCrT5",
                "scaledScore": 500
            },
            "levelMethod": "prescribed",
            "levelServiceID": "Nf13Nr3JoCd28tPnFJ6GCR",
            "levelServiceName": "V8vod"
        },
        "progressionLevel": "LWCrT5"
    }
}
```


