#!/bin/bash

LOG_FILE="/home/PWServer/logs/world2.log"

clear
echo -e "\033[1;34m🔍 Monitor Básico de Log iniciado\033[0m"
echo -e "\033[90mArquivo: $LOG_FILE\033[0m"
echo -e "\033[90m───────────────────────────────────────────────────────────────\033[0m"
echo -e "\033[1;33m📄 Exibindo logs em tempo real com conversão de caracteres chineses:\033[0m"
echo ""

# Monitoramento robusto com fallback: GB18030 > GBK > CP936
tail -f "$LOG_FILE" | while read -r line; do
    echo "$line" | iconv -f GB18030 -t UTF-8//IGNORE 2>/dev/null ||
    echo "$line" | iconv -f GBK -t UTF-8//IGNORE 2>/dev/null ||
    echo "$line" | iconv -f CP936 -t UTF-8//IGNORE 2>/dev/null ||
    echo "$line"
done
