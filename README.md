# Buddy 

**WORK IN PROGRESS**

Buddy synchronizes Google CloudDNS records with Google Compute Engine instances.

Design and implementation are inspired by [Mate](https://github.com/zalando-incubator/mate).

# Usage

* Project parameters:
  - google-project          : project ID that manages the DNS zone and compute resources 
  - google-zone             : name of the google compute zone to manage
  - google-region           : name of the google compute region to manage
  - external-ip-dns-zone    : default DNS managed zone name for external IPs
  - internal-ip-dns-zone    : default DNS managed zone name for internal IPs
  - dns-ttl                 : TTL in seconds for managed DNS resource records (default 300)
  - dns-zones               : comma separated names of DNS managed zones
  - multiple-ip-record      : allow multiple IP addresses in A record  (default true)
  - producer                : the endpoints producer to use (default google)    
  - consumer                : the endpoints consumer to use (default google)

* Instance metadata:
  - external-dns-zone       : Name of DNS managed zone for EXTERNAL_IP (A + TXT records).  
                              Value of project external-ip-dns-zone is used, when metadata value is empty 
  - internal-dns-zone       : Name of DNS managed zone for INTERNAL_IP (A + TXT records).
                              Value of project external-ip-dns-zone is used, when metadata value is empty
  - external-ip-hostname	: hostname in the external DNS zone or instance name when empty
  - internal-ip-hostname    : hostname in the internal DNS zone or instance name when empty 

* Instance tags - the same as instance metadata with empty value

For each tagged instance Buddy will create separate records for EXTERNAL_IP and INTERNAL_IP in the DNS zones:

1. A record - external or internal IP(s)
   
   DNS Name: `<hostname>.<Zone DNS Name>.`
   * hostname:
       - instance-name
       - internal-ip-hostname
       - external-ip-hostname
   * Zone DNS Name is a DNS Name in the configured managed DNS Zone   

2. TXT record - it has the same name as an A record. This helps to identify which records are created via Buddy
   
   TXT data: `buddy/<instance-compute-zone>/<instance-IPv4>`

# Examples
## Single instance

1. Create DNS Zones


    $ gcloud dns managed-zones create internal-example-com --description="DNS Zone for internal IPs" --dns-name=internal.example.org
    
    $ gcloud dns managed-zones create external-example-com --description="DNS Zone for external IPs" --dns-name=external.example.org

    $ gcloud dns managed-zones list

        NAME                     DNS_NAME                  DESCRIPTION
        external-example-com     external.example.org.     DNS Zone for external IPs
        internal-example-com     internal.example.org.     DNS Zone for internal IPs

    $ gcloud dns record-sets list -z internal-example-com

        NAME                   TYPE  TTL    DATA
        internal.example.org.  NS    21600  ns-cloud-a1.googledomains.com.,ns-cloud-a2.googledomains.com.,ns-cloud-a3.googledomains.com.,ns-cloud-a4.googledomains.com.
        internal.example.org.  SOA   21600  ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300

    $ gcloud dns record-sets list -z external-example-com

        NAME                   TYPE  TTL    DATA
        external.example.org.  NS    21600  ns-cloud-d1.googledomains.com.,ns-cloud-d2.googledomains.com.,ns-cloud-d3.googledomains.com.,ns-cloud-d4.googledomains.com.
        external.example.org.  SOA   21600  ns-cloud-d1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300

2. Start buddy


    $ buddy --google-project=my-project --google-zone=europe-west1-c --internal-ip-dns-zone=internal-example-com --external-ip-dns-zone=external-example-com --sync-interval=15

3. Create compute Engine with tag internal-ip-dns-zone and check DNS


    $ gcloud compute --project "my-project" instances create "instance-1" --zone "europe-west1-c" --machine-type "f1-micro" --tags "internal-ip-dns-zone"

        NAME        ZONE            MACHINE_TYPE  PREEMPTIBLE  INTERNAL_IP  EXTERNAL_IP    STATUS
        instance-1  europe-west1-c  f1-micro                   10.132.0.2   104.155.9.197  RUNNING


    Buddy logs
        
        INFO[2017-03-09T23:08:41+01:00] [Synchronize] Synchronizing DNS entries...
        INFO[2017-03-09T23:08:43+01:00] [Cloud DNS]: Change addition: instance-1.internal.example.org. / [10.132.0.2]


    $ gcloud dns record-sets list -z internal-example-com

        NAME                              TYPE  TTL    DATA
        internal.example.org.             NS    21600  ns-cloud-a1.googledomains.com.,ns-cloud-a2.googledomains.com.,ns-cloud-a3.googledomains.com.,ns-cloud-a4.googledomains.com.
        internal.example.org.             SOA   21600  ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300
        instance-1.internal.example.org.  A     300    10.132.0.2
        instance-1.internal.example.org.  TXT   300    "buddy/europe-west1-c/10.132.0.2"

4. Add additional tag external-ip-dns-zone and check DNS


    $ gcloud compute instances add-tags --tags=external-ip-dns-zone --zone=europe-west1-c instance-1

    Buddy logs
    
        INFO[2017-03-09T23:12:42+01:00] [Synchronize] Synchronizing DNS entries...
        INFO[2017-03-09T23:12:44+01:00] [Cloud DNS]: Change addition: instance-1.external.example.org. / [104.155.9.197]


    $ gcloud dns record-sets list -z external-example-com
    
        NAME                              TYPE  TTL    DATA
        external.example.org.             NS    21600  ns-cloud-d1.googledomains.com.,ns-cloud-d2.googledomains.com.,ns-cloud-d3.googledomains.com.,ns-cloud-d4.googledomains.com.
        external.example.org.             SOA   21600  ns-cloud-d1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300
        instance-1.external.example.org.  A     300    104.155.9.197
        instance-1.external.example.org.  TXT   300    "buddy/europe-west1-c/104.155.9.197"

5. Delete instance and check DNS


    $ gcloud compute instances delete --zone=europe-west1-c instance-1

    Buddy logs
    
        INFO[2017-03-09T23:19:29+01:00] [Cloud DNS]: Change deletion: instance-1.external.example.org. / [104.155.9.197]
        INFO[2017-03-09T23:19:29+01:00] [Cloud DNS]: Change deletion: instance-1.internal.example.org. / [10.132.0.2]

## Instance group


1. Create instance template with internal-ip-hostname and external-ip-hostname metadata
 
 
    $ gcloud compute --project "my-project" instance-templates create "instance-template-1" --machine-type "f1-micro" --metadata "internal-ip-hostname=my-internal-hostname,external-ip-hostname=my-external-hostname"


2. Create instance group with 2 instances 


    $ gcloud compute --project "my-project" instance-groups managed create "instance-group-1" --zone "europe-west1-c" --template "instance-template-1" --size "2"

    $ gcloud compute instances list
    
        NAME                   ZONE            MACHINE_TYPE  PREEMPTIBLE  INTERNAL_IP  EXTERNAL_IP     STATUS
        instance-group-1-c7h6  europe-west1-c  f1-micro                   10.132.0.3   130.211.107.47  RUNNING
        instance-group-1-zd4d  europe-west1-c  f1-micro                   10.132.0.2   104.155.9.197   RUNNING
 
3. Check DNS records have 2 IPs


    Buddy logs
    
        INFO[2017-03-09T23:36:32+01:00] [Synchronize] Synchronizing DNS entries...   
        INFO[2017-03-09T23:36:33+01:00] [Cloud DNS]: Change addition: my-internal-hostname.internal.example.org. / [10.132.0.3 10.132.0.2] 
        INFO[2017-03-09T23:36:33+01:00] [Cloud DNS]: Change addition: my-external-hostname.external.example.org. / [130.211.107.47 104.155.9.197] 


    $ gcloud dns record-sets list -z internal-example-com

        NAME                                        TYPE  TTL    DATA
        internal.example.org.                       NS    21600  ns-cloud-a1.googledomains.com.,ns-cloud-a2.googledomains.com.,ns-cloud-a3.googledomains.com.,ns-cloud-a4.googledomains.com.
        internal.example.org.                       SOA   21600  ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300
        my-internal-hostname.internal.example.org.  A     300    10.132.0.3,10.132.0.2
        my-internal-hostname.internal.example.org.  TXT   300    "buddy/europe-west1-c/10.132.0.3","buddy/europe-west1-c/10.132.0.2"

    
    $ gcloud dns record-sets list -z external-example-com
    
        NAME                                        TYPE  TTL    DATA
        external.example.org.                       NS    21600  ns-cloud-d1.googledomains.com.,ns-cloud-d2.googledomains.com.,ns-cloud-d3.googledomains.com.,ns-cloud-d4.googledomains.com.
        external.example.org.                       SOA   21600  ns-cloud-d1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300
        my-external-hostname.external.example.org.  A     300    130.211.107.47,104.155.9.197
        my-external-hostname.external.example.org.  TXT   300    "buddy/europe-west1-c/130.211.107.47","buddy/europe-west1-c/104.155.9.197"

4. Scale up
    
    
    $ gcloud compute --project "my-project" instance-groups managed resize "instance-group-1" --zone "europe-west1-c" --size=3


    Buddy logs
        
        INFO[2017-03-09T23:44:22+01:00] [Synchronize] Synchronizing DNS entries...   
        INFO[2017-03-09T23:44:23+01:00] [Cloud DNS]: Change modification: my-external-hostname.external.example.org. / [130.211.107.47 104.155.9.197] -> [130.211.107.47 130.211.63.173 104.155.9.197] 
        INFO[2017-03-09T23:44:23+01:00] [Cloud DNS]: Change modification: my-internal-hostname.internal.example.org. / [10.132.0.3 10.132.0.2] -> [10.132.0.3 10.132.0.4 10.132.0.2] 
    
    
    $ gcloud dns record-sets list -z internal-example-com
    
        NAME                                        TYPE  TTL    DATA
        internal.example.org.                       NS    21600  ns-cloud-a1.googledomains.com.,ns-cloud-a2.googledomains.com.,ns-cloud-a3.googledomains.com.,ns-cloud-a4.googledomains.com.
        internal.example.org.                       SOA   21600  ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300
        my-internal-hostname.internal.example.org.  A     300    10.132.0.3,10.132.0.4,10.132.0.2
        my-internal-hostname.internal.example.org.  TXT   300    "buddy/europe-west1-c/10.132.0.3","buddy/europe-west1-c/10.132.0.4","buddy/europe-west1-c/10.132.0.2"
    
    
    $ gcloud dns record-sets list -z external-example-com
    
        NAME                                        TYPE  TTL    DATA
        external.example.org.                       NS    21600  ns-cloud-d1.googledomains.com.,ns-cloud-d2.googledomains.com.,ns-cloud-d3.googledomains.com.,ns-cloud-d4.googledomains.com.
        external.example.org.                       SOA   21600  ns-cloud-d1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300
        my-external-hostname.external.example.org.  A     300    130.211.107.47,130.211.63.173,104.155.9.197
        my-external-hostname.external.example.org.  TXT   300    "buddy/europe-west1-c/130.211.107.47","buddy/europe-west1-c/130.211.63.173","buddy/europe-west1-c/104.155.9.197"

5. Scale down

    
    $ gcloud compute --project "my-project" instance-groups managed resize "instance-group-1" --zone "europe-west1-c" --size=2
    
    
    Buddy logs
         
        INFO[2017-03-09T23:47:04+01:00] [Synchronize] Synchronizing DNS entries...   
        INFO[2017-03-09T23:47:05+01:00] [Cloud DNS]: Change modification: my-external-hostname.external.example.org. / [130.211.107.47 130.211.63.173 104.155.9.197] -> [130.211.63.173 104.155.9.197] 
        INFO[2017-03-09T23:47:21+01:00] [Synchronize] Synchronizing DNS entries...   
        INFO[2017-03-09T23:47:22+01:00] [Cloud DNS]: Change modification: my-internal-hostname.internal.example.org. / [10.132.0.3 10.132.0.4 10.132.0.2] -> [10.132.0.4 10.132.0.2] 
    
5. Delete instance group  

  
    $ gcloud compute --project "my-project" instance-groups managed delete "instance-group-1"
    
    
    Buddy logs
         
        INFO[2017-03-09T23:49:29+01:00] [Synchronize] Synchronizing DNS entries...   
        INFO[2017-03-09T23:49:31+01:00] [Cloud DNS]: Change deletion: my-external-hostname.external.example.org. / [130.211.63.173 104.155.9.197] 
        INFO[2017-03-09T23:49:46+01:00] [Synchronize] Synchronizing DNS entries...   
        INFO[2017-03-09T23:49:47+01:00] [Cloud DNS]: Change deletion: my-internal-hostname.internal.example.org. / [10.132.0.4 10.132.0.2] 

