{130.149.249.182:8050 130.149.249.183:8050 130.149.249.184:8050} 
-> fork_tag(tag=vm, regex=true) { 
  '.+' 
  -> fork_tag(tag = "component") { 
    "*" 
    -> multiplex() { 
      "1" 
      -> empty://-; 
      "2" 
      -> '/home/vagrant/ceph-data-collector/OpenstackLogs/${component}.csv'
    } 
  };
  '^$' 
  -> empty://- 
}
-> :9001