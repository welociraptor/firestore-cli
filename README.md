# firestore-cli
(Yet another) command line interface for Google Cloud Firestore

## Installation
```bash
go mod vendor && go install
```

## Usage
```bash
command line interface for interaction with google cloud firestore

Usage:
  firestore-cli [command]

Available Commands:
  documents   return all documents in a collection
  get         get a document by id
  help        Help about any command
  where       query for documents

Flags:
  -c, --collection string   collection path
  -h, --help                help for firestore-cli
  -p, --prettyprint         pretty print document json
      --project string      gcp project id
  -v, --verbose             verbose mode

Use "firestore-cli [command] --help" for more information about a command.
```

## Contributing
Pull requests are welcome.

## License
[MIT](https://github.com/welociraptor/firestore-cli/blob/master/LICENSE)
