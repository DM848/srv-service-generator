# Service Generator

Creates a github repo with service discovery out of the box such that you can focusing on the actual code instead of communicating with Consul. Also includes healthchecking, logging interface, KV storage access to consule.

To create a new service send a POST request to this service behind the ACL service, example payload for a hello-world service running as an exposed endpoint.

```
curl \
  --request POST \
  --data '
    {
      "name": "hello-world",
      "public": true,
      "author": "my name"
    }' \
    --url http://our-platform-domain/api/service-generator/service
```

You must provide the "namme" and "author" fields. Public is optional, but must be set if you want your service exposed. Here are all the fields you can add:
```
  "name": "name of service", // required - only letters, numbers and dashes are allowed(!)
  "author": "name", // required -who ever deployedd or is responsible for maintenance
  
  // all of these are optional. Default values given.
  "port": 8888, // default value
  "desc": "description of the service", // appears in README.md
  "public": false, // true => accessable endpoint through the ACL gateway
  "replicas": 1, // pod instances
  "lang": "jolie", // programming language (must match a template repo suffix - [golang])
  "tags": [] // js array of string tags. These can be seen by other services.
```
