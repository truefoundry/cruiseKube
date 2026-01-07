# CruiseKube helm chart
CruiseKube is an intelligent Kubernetes resource optimization controller that automatically monitors, analyzes, and applies resource recommendations to improve cluster efficiency and reduce costs.

## Parameters

### Global parameters

| Name                              | Description              | Value                   |
| --------------------------------- | ------------------------ | ----------------------- |
| `global.postgresql.auth.host`     | PostgreSQL hostname      | `cruisekube-postgresql` |
| `global.postgresql.auth.port`     | PostgreSQL port number   | `5432`                  |
| `global.postgresql.auth.username` | PostgreSQL username      | `cruisekube`            |
| `global.postgresql.auth.password` | PostgreSQL password      | `cruisekube`            |
| `global.postgresql.auth.database` | PostgreSQL database name | `cruisekube`            |

### CruiseKube Controller parameters

| Name                                                        | Description                                                                                                           | Value                                |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------ |
| `cruisekubeController.enabled`                              | Enable CruiseKube Controller deployment                                                                               | `true`                               |
| `cruisekubeController.replicas`                             | Number of CruiseKube Controller replicas to deploy                                                                    | `1`                                  |
| `cruisekubeController.image.repository`                     | CruiseKube Controller image repository                                                                                | `tfy.jfrog.io/tfy-images/cruisekube` |
| `cruisekubeController.image.imagePullPolicy`                | CruiseKube Controller image pull policy                                                                               | `IfNotPresent`                       |
| `cruisekubeController.image.tag`                            | CruiseKube Controller image tag (immutable tags are recommended)                                                      | `0.1.8`                              |
| `cruisekubeController.imagePullSecrets`                     | CruiseKube Controller image pull secrets                                                                              | `[]`                                 |
| `cruisekubeController.nameOverride`                         | String to partially override cruisekube.fullname                                                                      | `""`                                 |
| `cruisekubeController.fullnameOverride`                     | String to fully override cruisekube.fullname                                                                          | `""`                                 |
| `cruisekubeController.controller.mode`                      | Controller mode (inCluster or external)                                                                               | `in-cluster`                         |
| `cruisekubeController.serviceAccount.create`                | Specifies whether a ServiceAccount should be created                                                                  | `true`                               |
| `cruisekubeController.serviceAccount.name`                  | The name of the ServiceAccount to use                                                                                 | `""`                                 |
| `cruisekubeController.rbac.create`                          | Specifies whether RBAC resources should be created                                                                    | `true`                               |
| `cruisekubeController.persistence.enabled`                  | Enable persistence using Persistent Volume Claims                                                                     | `true`                               |
| `cruisekubeController.persistence.size`                     | Persistent Volume size                                                                                                | `5Gi`                                |
| `cruisekubeController.persistence.storageClass`             | Persistent Volume storage class                                                                                       | `""`                                 |
| `cruisekubeController.persistence.accessMode`               | Persistent Volume access mode                                                                                         | `ReadWriteOnce`                      |
| `cruisekubeController.persistence.mountPath`                | Persistent Volume mount path                                                                                          | `/app/stats-data`                    |
| `cruisekubeController.persistence.annotations`              | Additional custom annotations for the PVC                                                                             | `{}`                                 |
| `cruisekubeController.podAnnotations`                       | Annotations for CruiseKube Controller pods                                                                            | `{}`                                 |
| `cruisekubeController.podLabels`                            | Extra labels for CruiseKube Controller pods                                                                           | `{}`                                 |
| `cruisekubeController.service.type`                         | CruiseKube Controller service type                                                                                    | `ClusterIP`                          |
| `cruisekubeController.service.httpPort`                     | CruiseKube Controller service HTTP port                                                                               | `8080`                               |
| `cruisekubeController.service.metricsPort`                  | CruiseKube Controller service metrics port                                                                            | `8081`                               |
| `cruisekubeController.service.annotations`                  | Additional custom annotations for the service                                                                         | `{}`                                 |
| `cruisekubeController.serviceMonitor.enabled`               | Create ServiceMonitor resource(s) for scraping metrics using PrometheusOperator                                       | `true`                               |
| `cruisekubeController.serviceMonitor.jobLabel`              | The name of the label on the target service to use as the job name in Prometheus                                      | `cruisekube-controller`              |
| `cruisekubeController.serviceMonitor.additionalLabels`      | Additional labels that can be used so ServiceMonitor resource(s) can be discovered by Prometheus                      | `{}`                                 |
| `cruisekubeController.serviceMonitor.additionalAnnotations` | Additional annotations for the ServiceMonitor resource(s)                                                             | `{}`                                 |
| `cruisekubeController.serviceMonitor.endpoints`             | ServiceMonitor endpoints configuration                                                                                | `[]`                                 |
| `cruisekubeController.resources.limits.cpu`                 | The resources limits (CPU) for the CruiseKube Controller containers                                                   | `500m`                               |
| `cruisekubeController.resources.limits.memory`              | The resources limits (memory) for the CruiseKube Controller containers                                                | `512Mi`                              |
| `cruisekubeController.resources.requests.cpu`               | The requested resources (CPU) for the CruiseKube Controller containers                                                | `100m`                               |
| `cruisekubeController.resources.requests.memory`            | The requested resources (memory) for the CruiseKube Controller containers                                             | `128Mi`                              |
| `cruisekubeController.volumeMounts`                         | Optionally specify extra list of additional volumeMounts for the CruiseKube Controller container(s)                   | `[]`                                 |
| `cruisekubeController.env`                                  | Environment variables to configure CruiseKube Controller. It is used to update the default values in the config file. | `{}`                                 |

### CruiseKube Webhook parameters

| Name                                                                                 | Description                                                                                      | Value                                                |
| ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ | ---------------------------------------------------- |
| `cruisekubeWebhook.enabled`                                                          | Enable CruiseKube Webhook deployment                                                             | `true`                                               |
| `cruisekubeWebhook.replicas`                                                         | Number of CruiseKube Webhook replicas to deploy                                                  | `1`                                                  |
| `cruisekubeWebhook.image.repository`                                                 | CruiseKube Webhook image repository                                                              | `tfy.jfrog.io/tfy-images/cruisekube`                 |
| `cruisekubeWebhook.image.imagePullPolicy`                                            | CruiseKube Webhook image pull policy                                                             | `IfNotPresent`                                       |
| `cruisekubeWebhook.image.tag`                                                        | CruiseKube Webhook image tag (immutable tags are recommended)                                    | `0.1.8`                                              |
| `cruisekubeWebhook.imagePullSecrets`                                                 | CruiseKube Webhook image pull secrets                                                            | `[]`                                                 |
| `cruisekubeWebhook.nameOverride`                                                     | String to partially override cruisekube.fullname                                                 | `""`                                                 |
| `cruisekubeWebhook.fullnameOverride`                                                 | String to fully override cruisekube.fullname                                                     | `""`                                                 |
| `cruisekubeWebhook.webhook.certsDir`                                                 | Directory path for webhook certificates                                                          | `/certs`                                             |
| `cruisekubeWebhook.webhook.statsURL.host`                                            | Stats URL host for webhook                                                                       | `""`                                                 |
| `cruisekubeWebhook.mutatingWebhookConfiguration.clusterID`                           | Cluster ID for mutating webhook configuration                                                    | `default`                                            |
| `cruisekubeWebhook.mutatingWebhookConfiguration.timeoutSeconds`                      | Timeout in seconds for webhook calls                                                             | `10`                                                 |
| `cruisekubeWebhook.mutatingWebhookConfiguration.failurePolicy`                       | Failure policy for webhook (Ignore or Fail)                                                      | `Ignore`                                             |
| `cruisekubeWebhook.mutatingWebhookConfiguration.namespaceSelector.excludeNamespaces` | Namespaces to exclude from webhook processing                                                    | `[]`                                                 |
| `cruisekubeWebhook.certGen.enabled`                                                  | Enable certificate generation for webhook                                                        | `true`                                               |
| `cruisekubeWebhook.certGen.image.repository`                                         | Certificate generator image repository                                                           | `registry.k8s.io/ingress-nginx/kube-webhook-certgen` |
| `cruisekubeWebhook.certGen.image.imagePullPolicy`                                    | Certificate generator image pull policy                                                          | `IfNotPresent`                                       |
| `cruisekubeWebhook.certGen.image.tag`                                                | Certificate generator image tag                                                                  | `v1.5.2`                                             |
| `cruisekubeWebhook.certGen.resources.limits.cpu`                                     | The resources limits (CPU) for the certificate generator containers                              | `100m`                                               |
| `cruisekubeWebhook.certGen.resources.limits.memory`                                  | The resources limits (memory) for the certificate generator containers                           | `128Mi`                                              |
| `cruisekubeWebhook.certGen.resources.requests.cpu`                                   | The requested resources (CPU) for the certificate generator containers                           | `10m`                                                |
| `cruisekubeWebhook.certGen.resources.requests.memory`                                | The requested resources (memory) for the certificate generator containers                        | `32Mi`                                               |
| `cruisekubeWebhook.certGen.rbac.create`                                              | Specifies whether RBAC resources should be created for certificate generator                     | `true`                                               |
| `cruisekubeWebhook.certGen.serviceAccount.create`                                    | Specifies whether a ServiceAccount should be created for certificate generator                   | `true`                                               |
| `cruisekubeWebhook.certGen.serviceAccount.name`                                      | The name of the ServiceAccount to use for certificate generator                                  | `""`                                                 |
| `cruisekubeWebhook.rbac.create`                                                      | Specifies whether RBAC resources should be created                                               | `true`                                               |
| `cruisekubeWebhook.podAnnotations`                                                   | Annotations for CruiseKube Webhook pods                                                          | `{}`                                                 |
| `cruisekubeWebhook.podLabels`                                                        | Extra labels for CruiseKube Webhook pods                                                         | `{}`                                                 |
| `cruisekubeWebhook.serviceAccount.create`                                            | Specifies whether a ServiceAccount should be created                                             | `true`                                               |
| `cruisekubeWebhook.serviceAccount.name`                                              | The name of the ServiceAccount to use                                                            | `""`                                                 |
| `cruisekubeWebhook.service.type`                                                     | CruiseKube Webhook service type                                                                  | `ClusterIP`                                          |
| `cruisekubeWebhook.service.httpsPort`                                                | CruiseKube Webhook service HTTPS port                                                            | `8443`                                               |
| `cruisekubeWebhook.service.metricsPort`                                              | CruiseKube Webhook service metrics port                                                          | `8081`                                               |
| `cruisekubeWebhook.service.annotations`                                              | Additional custom annotations for the service                                                    | `{}`                                                 |
| `cruisekubeWebhook.serviceMonitor.enabled`                                           | Create ServiceMonitor resource(s) for scraping metrics using PrometheusOperator                  | `true`                                               |
| `cruisekubeWebhook.serviceMonitor.jobLabel`                                          | The name of the label on the target service to use as the job name in Prometheus                 | `cruisekube-webhook`                                 |
| `cruisekubeWebhook.serviceMonitor.additionalLabels`                                  | Additional labels that can be used so ServiceMonitor resource(s) can be discovered by Prometheus | `{}`                                                 |
| `cruisekubeWebhook.serviceMonitor.additionalAnnotations`                             | Additional annotations for the ServiceMonitor resource(s)                                        | `{}`                                                 |
| `cruisekubeWebhook.serviceMonitor.endpoints`                                         | ServiceMonitor endpoints configuration                                                           | `[]`                                                 |
| `cruisekubeWebhook.resources.limits.cpu`                                             | The resources limits (CPU) for the CruiseKube Webhook containers                                 | `500m`                                               |
| `cruisekubeWebhook.resources.limits.memory`                                          | The resources limits (memory) for the CruiseKube Webhook containers                              | `512Mi`                                              |
| `cruisekubeWebhook.resources.requests.cpu`                                           | The requested resources (CPU) for the CruiseKube Webhook containers                              | `100m`                                               |
| `cruisekubeWebhook.resources.requests.memory`                                        | The requested resources (memory) for the CruiseKube Webhook containers                           | `128Mi`                                              |
| `cruisekubeWebhook.env`                                                              | Environment variables to configure CruiseKube Webhook                                            | `{}`                                                 |

### PostgreSQL parameters

| Name                                           | Description                                                            | Value   |
| ---------------------------------------------- | ---------------------------------------------------------------------- | ------- |
| `postgresql.enabled`                           | Switch to enable or disable the PostgreSQL helm chart                  | `false` |
| `postgresql.primary.persistence.enabled`       | Enable PostgreSQL Primary data persistence using PVC                   | `true`  |
| `postgresql.primary.persistence.size`          | PVC Storage Request for PostgreSQL volume                              | `2Gi`   |
| `postgresql.primary.resources.limits.cpu`      | The resources limits (CPU) for the PostgreSQL Primary containers       | `500m`  |
| `postgresql.primary.resources.limits.memory`   | The resources limits (memory) for the PostgreSQL Primary containers    | `512Mi` |
| `postgresql.primary.resources.requests.cpu`    | The requested resources (CPU) for the PostgreSQL Primary containers    | `100m`  |
| `postgresql.primary.resources.requests.memory` | The requested resources (memory) for the PostgreSQL Primary containers | `128Mi` |

### CruiseKube Frontend parameters

| Name                                           | Description                                                             | Value                                         |
| ---------------------------------------------- | ----------------------------------------------------------------------- | --------------------------------------------- |
| `cruisekubeFrontend.enabled`                   | Enable CruiseKube Frontend deployment                                   | `true`                                        |
| `cruisekubeFrontend.replicas`                  | Number of CruiseKube Frontend replicas to deploy                        | `1`                                           |
| `cruisekubeFrontend.image.repository`          | CruiseKube Frontend image repository                                    | `tfy.jfrog.io/tfy-images/cruisekube-frontend` |
| `cruisekubeFrontend.image.imagePullPolicy`     | CruiseKube Frontend image pull policy                                   | `IfNotPresent`                                |
| `cruisekubeFrontend.image.tag`                 | CruiseKube Frontend image tag (immutable tags are recommended)          | `0.1.8`                                       |
| `cruisekubeFrontend.imagePullSecrets`          | CruiseKube Frontend image pull secrets                                  | `[]`                                          |
| `cruisekubeFrontend.nameOverride`              | String to partially override cruisekube.fullname                        | `""`                                          |
| `cruisekubeFrontend.fullnameOverride`          | String to fully override cruisekube.fullname                            | `""`                                          |
| `cruisekubeFrontend.podAnnotations`            | Annotations for CruiseKube Frontend pods                                | `{}`                                          |
| `cruisekubeFrontend.podLabels`                 | Extra labels for CruiseKube Frontend pods                               | `{}`                                          |
| `cruisekubeFrontend.service.type`              | CruiseKube Frontend service type                                        | `ClusterIP`                                   |
| `cruisekubeFrontend.service.httpPort`          | CruiseKube Frontend service HTTP port                                   | `3000`                                        |
| `cruisekubeFrontend.service.annotations`       | Additional custom annotations for the service                           | `{}`                                          |
| `cruisekubeFrontend.backendURL`                | Backend URL for the frontend to connect to                              | `""`                                          |
| `cruisekubeFrontend.resources.limits.cpu`      | The resources limits (CPU) for the CruiseKube Frontend containers       | `200m`                                        |
| `cruisekubeFrontend.resources.limits.memory`   | The resources limits (memory) for the CruiseKube Frontend containers    | `256Mi`                                       |
| `cruisekubeFrontend.resources.requests.cpu`    | The requested resources (CPU) for the CruiseKube Frontend containers    | `50m`                                         |
| `cruisekubeFrontend.resources.requests.memory` | The requested resources (memory) for the CruiseKube Frontend containers | `64Mi`                                        |
| `cruisekubeFrontend.env`                       | Environment variables to configure CruiseKube Frontend                  | `{}`                                          |
