# Create login session
curl --request POST \
  --url https://cloud2.viettelidc.com.vn/api/sessions \
  --header 'accept: application/*+json;version=37.0' \
  --header 'authorization: Basic bmdoaWFsdEBzeXN0ZW06TGUwOTAzOTA4Mjg1Iw=='
---
Response:
| Header | Value |
| --- | --- |
| x-vmware-vcloud-access-token | eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJhOTNjOWRiOS03NDcxLTMxOTItOGQwOS1hOGY3ZWVkYTg1ZjlAMTdlODFjNDUtYTE2ZC00MWNlLWJlZjQtNmQ3YTBkNjIxMDMwIiwic3ViIjoibmdoaWFsdCIsImV4cCI6MTc3OTk0NzA4MSwidmVyc2lvbiI6InZjbG91ZF8xLjAiLCJqdGkiOiI2MTExZDA3MjEyNmE0ZWMxODZmYWRiYWRkMjAzZGY0OCJ9.B40cBNUNVHXQrkvA0LrTr27_DynmKGYDsuTXPLjQFTxuEbYzvxpAPqq8dr8Mttp-lBm-JP69PxLtigbaeLA6CIUBksn606CyidJDocqzV3JjcqL85aKzXRhp-hhB5n_dG_7Kye_G8FM3TdBm5vEBNEEHZz8DWB53KanygdlZR6MVh_A-0xrVD_hROpZB_SHgHyXekTrZCdwaAnh4gsP1NaRKKkNP11zfcysumplr2R3lItOx1N4WFAkVttD3ohHZfXlcSOXWh1U5Fes-TgsFaOemxVA25G8BJIghh9fh6Rt4caskieha3XvUVI0MP2fzK3VN50jVSWUEH1CZey732g |
| x-vmware-vcloud-token-type | Bearer |
| x-vcloud-authorization | 6111d072126a4ec186fadbadd203df48 |
----

# Get EdgeGateway Configuration
curl 'https://cloud2.viettelidc.com.vn/api/admin/edgeGateway/011430c1-e8f7-467e-a210-43be1c3922f2' \
  -H 'User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:151.0) Gecko/20100101 Firefox/151.0' \
  -H 'Accept: application/*+xml;version=37.2' \
  -H 'Accept-Language: en-US,en;q=0.9' \
  -H 'Accept-Encoding: gzip, deflate, br, zstd' \
  -H 'x-requested-with: XMLHttpRequest' \
  -H 'x-vcloud-authorization: 67146c467bdc4ba385a5bc2071b5041b' \
  -H 'Connection: keep-alive' \
  -H 'Referer: https://cloud2.viettelidc.com.vn/tenant-networking/network-edges/011430c1-e8f7-467e-a210-43be1c3922f2?org=ATM-6600831' \
  -H 'Cookie: _gcl_au=1.1.1097660932.1773304775; _ga_5K2VWVZ7ZJ=GS2.1.s1773304774$o1$g0$t1773304780$j54$l0$h0; _ga=GA1.1.2014956292.1773304775; _ga_MJQP8Q61LQ=GS2.1.s1773304775$o1$g0$t1773304780$j55$l0$h0; _ga_6DQMVK2H8W=GS2.1.s1773304775$o1$g0$t1773304780$j55$l0$h0; sc_is_visitor_unique=rx12775897.1773304776.5BE34E0F4ED14A958F941ECF3DC5DE0B.1.1.1.1.1.1.1.1.1; _fbp=fb.2.1773304777683.301744555697242921; _clck=nbeicv%5E2%5Eg4a%5E0%5E2262; messagesUtk=4384b305e23c4cf4b1b2fa4d257583c4; __hstc=41773373.bc5200e376b5213e28a948d46eb4db8b.1773304780264.1773304780264.1773304780264.1; hubspotutk=bc5200e376b5213e28a948d46eb4db8b; vcloud_jwt=eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJhOTNjOWRiOS03NDcxLTMxOTItOGQwOS1hOGY3ZWVkYTg1ZjlAMTdlODFjNDUtYTE2ZC00MWNlLWJlZjQtNmQ3YTBkNjIxMDMwIiwic3ViIjoibmdoaWFsdCIsImV4cCI6MTc3OTkzMDkyMCwidmVyc2lvbiI6InZjbG91ZF8xLjAiLCJqdGkiOiI2NzE0NmM0NjdiZGM0YmEzODVhNWJjMjA3MWI1MDQxYiJ9.lWrlgthVPsx3-5iI48BJ9W23TL4yQbImm9cDUj2e85zNuXcAlCClB0jBudb6lvhh6mxaJIQ-JL015RG7SI2NWn5oGbg4BHH9377fKLEQrbuMUun1feFMQQvEvcASNXrMKB1twn4nDgYZho1HDb0dMMtF8ha9rv5CUcOv2d8k7brKQtv_QIJgy0HYvVui2T6nC3gKCpP2n2FU8wHkQPPJnofK8GJJMt2utq7HBwEGAghfGlr4uulObRE4ExJmUu2wBgRzTXTpGUC8-mgfp0jBJrsJg0IozFTKcv3mlmeUxMyKXyNJ_CfVtEwaQBMq-1JHveno0na0kmAln2K8pjfGkg; vcloud_session_id=67146c467bdc4ba385a5bc2071b5041b' \
  -H 'Sec-Fetch-Dest: empty' \
  -H 'Sec-Fetch-Mode: cors' \
  -H 'Sec-Fetch-Site: same-origin' \
  -H 'Priority: u=0' \
  -H 'Pragma: no-cache' \
  -H 'Cache-Control: no-cache'
  
 # Get Edge IPSec config
 curl 'https://cloud2.viettelidc.com.vn/network/edges/011430c1-e8f7-467e-a210-43be1c3922f2/ipsec/config?showSensitiveData=true' \
  -H 'User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:151.0) Gecko/20100101 Firefox/151.0' \
  -H 'Accept: application/xml' \
  -H 'Accept-Language: en-US,en;q=0.9' \
  -H 'Accept-Encoding: gzip, deflate, br, zstd' \
  -H 'x-requested-with: XMLHttpRequest' \
  -H 'x-vcloud-authorization: 67146c467bdc4ba385a5bc2071b5041b' \
  -H 'Connection: keep-alive' \
  -H 'Referer: https://cloud2.viettelidc.com.vn/tenant-networking/network-edges/011430c1-e8f7-467e-a210-43be1c3922f2?org=ATM-6600831' \
  -H 'Cookie: _gcl_au=1.1.1097660932.1773304775; _ga_5K2VWVZ7ZJ=GS2.1.s1773304774$o1$g0$t1773304780$j54$l0$h0; _ga=GA1.1.2014956292.1773304775; _ga_MJQP8Q61LQ=GS2.1.s1773304775$o1$g0$t1773304780$j55$l0$h0; _ga_6DQMVK2H8W=GS2.1.s1773304775$o1$g0$t1773304780$j55$l0$h0; sc_is_visitor_unique=rx12775897.1773304776.5BE34E0F4ED14A958F941ECF3DC5DE0B.1.1.1.1.1.1.1.1.1; _fbp=fb.2.1773304777683.301744555697242921; _clck=nbeicv%5E2%5Eg4a%5E0%5E2262; messagesUtk=4384b305e23c4cf4b1b2fa4d257583c4; __hstc=41773373.bc5200e376b5213e28a948d46eb4db8b.1773304780264.1773304780264.1773304780264.1; hubspotutk=bc5200e376b5213e28a948d46eb4db8b; vcloud_jwt=eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJhOTNjOWRiOS03NDcxLTMxOTItOGQwOS1hOGY3ZWVkYTg1ZjlAMTdlODFjNDUtYTE2ZC00MWNlLWJlZjQtNmQ3YTBkNjIxMDMwIiwic3ViIjoibmdoaWFsdCIsImV4cCI6MTc3OTkzMDkyMCwidmVyc2lvbiI6InZjbG91ZF8xLjAiLCJqdGkiOiI2NzE0NmM0NjdiZGM0YmEzODVhNWJjMjA3MWI1MDQxYiJ9.lWrlgthVPsx3-5iI48BJ9W23TL4yQbImm9cDUj2e85zNuXcAlCClB0jBudb6lvhh6mxaJIQ-JL015RG7SI2NWn5oGbg4BHH9377fKLEQrbuMUun1feFMQQvEvcASNXrMKB1twn4nDgYZho1HDb0dMMtF8ha9rv5CUcOv2d8k7brKQtv_QIJgy0HYvVui2T6nC3gKCpP2n2FU8wHkQPPJnofK8GJJMt2utq7HBwEGAghfGlr4uulObRE4ExJmUu2wBgRzTXTpGUC8-mgfp0jBJrsJg0IozFTKcv3mlmeUxMyKXyNJ_CfVtEwaQBMq-1JHveno0na0kmAln2K8pjfGkg; vcloud_session_id=67146c467bdc4ba385a5bc2071b5041b' \
  -H 'Sec-Fetch-Dest: empty' \
  -H 'Sec-Fetch-Mode: cors' \
  -H 'Sec-Fetch-Site: same-origin' \
  -H 'Priority: u=0' \
  -H 'Pragma: no-cache' \
  -H 'Cache-Control: no-cache'
