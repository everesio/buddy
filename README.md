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

