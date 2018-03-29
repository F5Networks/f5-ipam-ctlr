F5 IPAM Controller
==================

.. toctree::
   :hidden:
   :maxdepth: 2

   Home <self>
   RELEASE-NOTES
   _static/ATTRIBUTIONS.md

Overview
--------

The |ctlr-long| is a Docker container that runs in an orchestration environment and interfaces
with an IPAM system.
It lets you allocate IP addresses from an IPAM system's address pool for host names in an orchestration environment.
The Controller watches orchestration-specific resources and consumes the host names within each resource.

The Controller can:

- Create A and CNAME DNS records in the IPAM system for the supplied host names, using the next
  available address in the specified subnet.
- Write an annotation or label with the selected IP address back to orchestration resources.
- Update A or CNAME records if a resource's host names get updated.
- Delete any A or CNAME records and release reserved IP addresses from deleted resources.

|release-notes|

|attributions|

:fonticon:`fa fa-download` :download:`Attributions.md </_static/ATTRIBUTIONS.md>`

Features
--------

- Interfaces with an orchestration environment to receive lists of host names.
- Interfaces with an IPAM system to allocate IP addresses for the requested host names.
- Creates A records and CNAME records for the host names and chosen IP addresses.
- Annotates the orchestration resources with the chosen IP addresses to enable integration
  with an `F5 Container Connector`_.

Supported Environments
----------------------

Orchestrations
``````````````

- `Kubernetes`_
- `OpenShift`_

IPAM systems
````````````

- `Infoblox`_

.. _configuration parameters:

Configuration Parameters
------------------------

.. tip:: See the :ref:`example configuration files <conf examples>` for usage examples.

.. _general configs:

General
```````

All configuration parameters below are global to the |ctlr|.

+-----------------+---------+----------+---------+----------------------------------------+----------------+
| Parameter       | Type    | Required | Default | Description                            | Allowed Values |
+=================+=========+==========+=========+========================================+================+
| log-level       | string  | Optional | INFO    | Log level                              | INFO,          |
|                 |         |          |         |                                        | DEBUG,         |
|                 |         |          |         |                                        | CRITICAL,      |
|                 |         |          |         |                                        | WARNING,       |
|                 |         |          |         |                                        | ERROR          |
+-----------------+---------+----------+---------+----------------------------------------+----------------+
| ip-manager      | string  | Required | n/a     | The IPAM system that the controller    | infoblox       |
|                 |         |          |         | will interface with.                   |                |
+-----------------+---------+----------+---------+----------------------------------------+----------------+
| orchestration   | string  | Required | n/a     | The orchestration that the controller  | kubernetes,    |
|                 |         |          |         | is running in.                         | k8s, openshift |
+-----------------+---------+----------+---------+----------------------------------------+----------------+
| verify-interval | integer | Optional | 30      | In seconds, the interval at which      |                |
|                 |         |          |         | to verify the IPAM configuration.      |                |
|                 |         |          |         |                                        |                |
|                 |         |          |         | Set to :code:`0` to disable.           |                |
+-----------------+---------+----------+---------+----------------------------------------+----------------+
| version         | string  | Optional | n/a     | Print the controller version and exit. |                |
+-----------------+---------+----------+---------+----------------------------------------+----------------+


IPAM Systems
````````````

.. _iblox configs:

Infoblox
~~~~~~~~

+-----------------------+---------+----------+---------+-----------------------------------------+
| Parameter             | Type    | Required | Default | Description                             |
+=======================+=========+==========+=========+=========================================+
| credentials-directory | string  | Optional | n/a     | Directory that contains the infoblox    |
|                       |         |          |         | username and password files.            |
+-----------------------+---------+----------+---------+-----------------------------------------+
| infoblox-grid-host    | string  | Required | n/a     | The grid manager host IP address.       |
+-----------------------+---------+----------+---------+-----------------------------------------+
| infoblox-password     | string  | Required | n/a     | The login password.                     |
+-----------------------+---------+----------+---------+-----------------------------------------+
| infoblox-port         | integer | Optional | 443     | The Web API port.                       |
+-----------------------+---------+----------+---------+-----------------------------------------+
| infoblox-username     | string  | Required | n/a     | The login username.                     |
+-----------------------+---------+----------+---------+-----------------------------------------+
| infoblox-wapi-version | string  | Required | n/a     | The Web API version.                    |
+-----------------------+---------+----------+---------+-----------------------------------------+

.. important::

   The :code:`credentials-directory` option is an alternative to using the :code:`infoblox-username` and :code:`infoblox-password` arguments.

   When you use this argument, the controller expects to find two files:

   - "infoblox-username" and
   - "infoblox-password"

   Each file should contain **only** the username and the password, respectively.
   You can create the files as `Kubernetes Secrets`_.

Orchestration
`````````````

.. _k8s-env configs:

Kubernetes
~~~~~~~~~~

+--------------------+---------+----------+----------+-------------------------------------+----------------+
| Parameter          | Type    | Required | Default  | Description                         | Allowed Values |
+====================+=========+==========+==========+=====================================+================+
| kubeconfig         | string  | Optional | ./config | Path to the *kubeconfig* file       |                |
+--------------------+---------+----------+----------+-------------------------------------+----------------+
| namespace          | string  | Optional | All      | Kubernetes namespace(s) to watch    |                |
|                    |         |          |          |                                     |                |
|                    |         |          |          | - may be a comma-separated list     |                |
|                    |         |          |          | - watches all namespaces by default |                |
+--------------------+---------+----------+----------+-------------------------------------+----------------+
| namespace-label    | string  | Optional | n/a      | Tells the |ctlr| to watch           |                |
|                    |         |          |          | any namespace with this label.      |                |
+--------------------+---------+----------+----------+-------------------------------------+----------------+
| running-in-cluster | boolean | Optional | true     | Indicates whether or not a          | true, false    |
|                    |         |          |          | kubernetes cluster started          |                |
|                    |         |          |          | |ctlr|.                             |                |
+--------------------+---------+----------+----------+-------------------------------------+----------------+

.. _installation:

Installation
------------

Take the steps below to install the |ctlr| in Kubernetes or OpenShift.

#. Set up RBAC as appropriate for your Cluster. The |ctlr| requires the following permissions:

   .. code-block:: yaml

      - apiGroups:
        - ""
        - "extensions"
        resources:
        - configmaps
        - ingresses
        verbs:
        - get
        - list
        - watch
        - update
        - patch
      - apiGroups:
        - ""
        resources:
        - namespaces
        verbs:
        - get
        - list
        - watch

   **Example:**

   To give the |ctlr| cluster-wide access to resources, define a `ServiceAccount`_, `ClusterRole`_, and `ClusterRole Binding`_.

   :ref:`View <rbac-example>` or :fonticon:`fa fa-download` :download:`download </_static/config_examples/f5-ipam-ctlr-rbac.yaml>` example RBAC resources

   .. tip:: Do not grant the |ctlr| more access than needed for your specific use case. If the |ctlr| will watch a specific namespace(s), consider using a Role and RoleBinding instead.

#. Define the :ref:`configuration parameters` in a Kubernetes Deployment using YAML or JSON.

   :ref:`View <deployment-example-basic>` or :fonticon:`fa fa-download` :download:`download </_static/config_examples/f5-ipam-ctlr.yaml>` a basic Deployment

   :ref:`View <deployment-example-credentials>` or :fonticon:`fa fa-download` :download:`download </_static/config_examples/f5-ipam-ctlr.yaml>` a Deployment that uses the :code:`credentials-directory`

#. Upload the resources to the Kubernetes or OpenShift API server.

   .. parsed-literal::

      kubectl create -f **f5-ipam-ctlr-rbac.yaml** -f **f5-ipam-ctlr.yaml** [-n <desired_namespace>]
      ______

      oc create -f **f5-ipam-ctlr-rbac.yaml** -f **f5-ipam-ctlr.yaml** [-n <desired_namespace>]


.. _usage:

Usage
-----

To use the |ctlr-long| in Kubernetes or OpenShift, add the resource annotations to a ConfigMap or Ingress resource.

.. important::

   Use of the resource annotations with OpenShift Routes is not supported.

.. rubric:: **Example**
.. parsed-literal::

   kubectl annotate ingress <ingress_name> **ipam.f5.com/infoblox-netview=default** **ipam.f5.com/ip-allocation=dynamic** **ipam.f5.com/network-cidr=1.2.3.0/24**

.. note::

   The |ctlr| writes the chosen IP address to each resource using the following annotation:

   :code:`virtual-server.f5.com/ip`

.. important::

   If changing the network view, network cidr, or group, we recommend deleting the Ingress or ConfigMap first,
   then perform the edits, and recreate the resource. Changing these fields "live" may cause unwanted behavior.


.. _k8s-resource configs:

Kubernetes Resource Annotations
```````````````````````````````

In Kubernetes, the |ctlr-long| watches for `ConfigMap`_ and `Ingress`_ resources with the required `annotations`_.
When using the |ctlr-long| with the `F5 BIG-IP Controller for Kubernetes`_, the |ctlr| can watch for `F5 Resource`_ ConfigMaps.

Add the Annotations shown in the table below to Kubernetes resources to manage IP address assignment with the |ctlr|.

+-----------------------------------+--------+----------+-----------------------------------------+----------------+
| Annotation                        | Type   | Required | Description                             | Supported      |
|                                   |        |          |                                         | Resource(s)    |
+===================================+========+==========+=========================================+================+
| ipam.f5.com/group                 | string | Optional | Assign a single IP address to a group   | multi-service  |
|                                   |        |          | of multi-service Ingress resources.     | Ingress        |
|                                   |        |          |                                         |                |
|                                   |        |          | Ungrouped multi-service Ingress         |                |
|                                   |        |          | resources receive unique (non-shared)   |                |
|                                   |        |          | IP addresses.                           |                |
+-----------------------------------+--------+----------+-----------------------------------------+----------------+
| ipam.f5.com/infoblox-netview      | string | Required | Specifies the Infoblox network          | ConfigMap,     |
|                                   |        |          | view in which to allocate the IP        | Ingress        |
|                                   |        |          | address.                                |                |
+-----------------------------------+--------+----------+-----------------------------------------+----------------+
| ipam.f5.com/ip-allocation=dynamic | string | Required | Tells the |ctlr| to watch this resource | ConfigMap,     |
|                                   |        |          | and allocate IP addresses for its hosts.| Ingress        |
+-----------------------------------+--------+----------+-----------------------------------------+----------------+
| ipam.f5.com/network-cidr          | string | Required | Specifies the subnet in which to        | ConfigMap,     |
|                                   |        |          | allocate the IP address.                | Ingress        |
+-----------------------------------+--------+----------+-----------------------------------------+----------------+
| ipam.f5.com/hostname              | string | Required | Specifies the hostname for which to     | ConfigMap,     |
|                                   |        |          | create a DNS record.                    | single-service |
|                                   |        |          |                                         | Ingress        |
+-----------------------------------+--------+----------+-----------------------------------------+----------------+

.. _conf examples:

Example Configuration Files
---------------------------

.. toctree::
   :caption: Config Examples
   :glob:
   :maxdepth: 1

   *-example
