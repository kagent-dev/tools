#!/bin/bash

ps -f | grep kubectl | grep port-forward | grep argocd-server | grep argocd | grep -v grep | awk '{print $2}' | xargs kill -9 || true
kubectl port-forward svc/argocd-server -n argocd 18080:443 &

#argocd.default.svc.cluster.local
argocd login 127.0.0.1:18080 \
  --username admin \
  --password $(kubectl get secret argocd-initial-admin-secret -n argocd -o jsonpath='{.data.password}' | base64 -d) \
  --insecure

argocd cluster list
