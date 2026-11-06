#!/usr/bin/env bash
# Shells and locales for FuzzFormatPosix on Ubuntu runners. TestLocalesExist
# fails on CI without them: the fuzz evals runlike's output in every shell
# and locale (EUC lead bytes). macOS ships them.
set -euo pipefail
sudo apt-get update -q
sudo apt-get install -y -q locales zsh dash
sudo localedef -i en_US -f UTF-8 en_US.UTF-8
sudo localedef -i ko_KR -f EUC-KR ko_KR.eucKR
sudo localedef -i ko_KR -f CP949 ko_KR.CP949
sudo localedef -i zh_CN -f GB2312 zh_CN.eucCN
sudo localedef -i zh_CN -f GB2312 zh_CN.GB2312
sudo localedef -i ja_JP -f EUC-JP ja_JP.eucJP
