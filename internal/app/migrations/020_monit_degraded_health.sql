UPDATE devices d SET status=CASE
  WHEN EXISTS(SELECT 1 FROM devices p WHERE p.id=d.parent_id AND p.status IN ('down','dependency')) THEN 'dependency'
  WHEN EXISTS(SELECT 1 FROM checks WHERE device_id=d.id AND status='down')
    OR EXISTS(SELECT 1 FROM monit_hosts mh JOIN monit_services ms ON ms.host_id=mh.id WHERE mh.device_id=d.id AND ms.monitor<>0 AND ms.status<>0) THEN 'down'
  WHEN EXISTS(SELECT 1 FROM monit_hosts mh JOIN monit_services ms ON ms.host_id=mh.id WHERE mh.device_id=d.id AND ms.monitor=0) THEN 'degraded'
  WHEN EXISTS(SELECT 1 FROM checks WHERE device_id=d.id AND status='up')
    OR EXISTS(SELECT 1 FROM monit_hosts WHERE device_id=d.id)
    OR EXISTS(SELECT 1 FROM agent_reports WHERE device_id=d.id) THEN 'up'
  ELSE 'unknown'
END, updated_at=now();
