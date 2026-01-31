#!/bin/bash

# konfigurasi eksperimen
# repetitions adalah jumlah pengulangan untuk setiap kombinasi
# image counts adalah variasi jumlah gambar yang diuji
REPETITIONS=5
IMAGE_COUNTS=(1 10 50 100)

# format skenario: nama|is_concurrent|use_worker_pool|num_workers
SCENARIOS=(
    "Sequential|false|false|1"
    "Naive_Concurrent|true|false|1"
    "WorkerPool_4|true|true|4"
)

GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${GREEN}Starting Experiment Automation...${NC}"
echo "Total Runs: ${#SCENARIOS[@]} Scenarios x ${#IMAGE_COUNTS[@]} Loads x $REPETITIONS Sets = $(( ${#SCENARIOS[@]} * ${#IMAGE_COUNTS[@]} * $REPETITIONS )) runs."
echo "--------------------------------------------------"

# build image docker sekali di awal
echo "Building Worker Image..."
docker-compose --compatibility build worker

# start rabbitmq sekali di awal agar metric persistent
echo "Starting RabbitMQ..."
docker-compose --compatibility up -d rabbitmq
echo "Waiting for RabbitMQ..."
sleep 20

# buat folder logs jika belum ada
mkdir -p results/logs

# inisialisasi file csv jika belum ada dan atur permission agar bisa ditulisi oleh user maupun docker (root)
if [ ! -f results/experiment_data.csv ]; then
    echo "Timestamp,Scenario,Total_Images,Duration_Sec,CPU_Avg_Percent,Peak_RAM_MB,Num_Workers" > results/experiment_data.csv
fi
chmod 666 results/experiment_data.csv

# loop utama untuk setiap skenario
for scenario in "${SCENARIOS[@]}"; do
    # parse konfigurasi dari string format
    IFS="|" read -r NAME IS_CONC USE_POOL WORKERS <<< "$scenario"
    
    echo -e "\n${CYAN}>>> SWITCHING TO SCENARIO: $NAME ${NC}"
    
    # update file env dengan konfigurasi skenario
    sed -i "s/^IS_CONCURRENT=.*/IS_CONCURRENT=$IS_CONC/" .env
    sed -i "s/^USE_WORKER_POOL=.*/USE_WORKER_POOL=$USE_POOL/" .env
    sed -i "s/^NUM_WORKERS=.*/NUM_WORKERS=$WORKERS/" .env
    
    sed -i "s/^USE_WORKER_POOL=.*/USE_WORKER_POOL=$USE_POOL/" .env
    sed -i "s/^NUM_WORKERS=.*/NUM_WORKERS=$WORKERS/" .env
    
    # alokasi cpu berbeda untuk sequential vs concurrent
    # sequential hanya 1 core sesuai metodologi
    if [ "$IS_CONC" = "false" ]; then
        export CPU_LIMIT="1.0"
    else
        export CPU_LIMIT="4.0"
    fi
    
    echo "Config updated: IsConc=$IS_CONC, UsePool=$USE_POOL, Workers=$WORKERS, CPU_LIMIT=$CPU_LIMIT"

    # loop untuk setiap variasi jumlah gambar
    for count in "${IMAGE_COUNTS[@]}"; do
        echo -e "\n${CYAN}  > Testing Load: $count Images${NC}"
        
        # set batch size sesuai jumlah gambar
        if grep -q "BATCH_SIZE=" .env; then
            sed -i "s/^BATCH_SIZE=.*/BATCH_SIZE=$count/" .env
        else
            echo "BATCH_SIZE=$count" >> .env
        fi
        
        # loop untuk pengulangan
        for (( i=1; i<=REPETITIONS; i++ )); do
            echo "    [Run $i/$REPETITIONS] Initializing..."

            # bersihkan hasil kompresi sebelumnya
            docker-compose --compatibility run --rm -v "$(pwd)/storage/compressed:/data" alpine sh -c "rm -rf /data/*" > /dev/null 2>&1
            
            # hapus kontainer worker dan seeder
            docker-compose rm -s -f -v worker seeder > /dev/null 2>&1
            
            # bersihkan queue sebelum tes dimulai
            docker-compose exec -T rabbitmq rabbitmqctl purge_queue image_tasks > /dev/null 2>&1
            
            # masukkan task ke queue
            echo "    Seeding $count images..."
            docker-compose --compatibility run --rm seeder -n $count
            
            echo "    Starting Worker..."
            LOG_FILE="results/logs/${NAME}_${count}_run${i}.log"
            
            # jalankan worker dan simpan log
            docker-compose --compatibility up --exit-code-from worker worker > "$LOG_FILE" 2>&1
            EXIT_CODE=$?
            
            cat "$LOG_FILE"
            
            # deteksi error dan oom
            if [ $EXIT_CODE -ne 0 ]; then
                echo -e "${CYAN}    [!] Worker exited with code $EXIT_CODE. Checking for OOM...${NC}"
                echo "    [!] Worker exited with code $EXIT_CODE. Checking for OOM..." >> "$LOG_FILE"
                
                # cek oom dari docker inspect
                IS_OOM=$(docker inspect exp_worker --format '{{.State.OOMKilled}}' 2>/dev/null)
                ERROR_MSG="FAILED_UNKNOWN"
                
                if [ "$IS_OOM" == "true" ]; then
                    ERROR_MSG="FAILED_OOM"
                    echo -e "${CYAN}    [INFO] OOM Detected (Source: Docker Inspect)${NC}"
                    echo "    [INFO] OOM Detected (Source: Docker Inspect)" >> "$LOG_FILE"
                elif [ $EXIT_CODE -eq 137 ]; then
                    # exit code 137 biasanya berarti killed by oom
                     ERROR_MSG="FAILED_OOM"
                     echo -e "${CYAN}    [INFO] OOM Detected (Source: Exit Code 137)${NC}"
                     echo "    [INFO] OOM Detected (Source: Exit Code 137)" >> "$LOG_FILE"
                else
                    ERROR_MSG="FAILED_ERROR_$EXIT_CODE"
                fi

                # tampilkan log kernel jika oom
                if [ "$ERROR_MSG" == "FAILED_OOM" ]; then
                     echo "    [INFO] Kernel OOM Logs:" | tee -a "$LOG_FILE"
                     dmesg | grep -E -i "oom|killed process|out of memory" | tail -n 5 | tee -a "$LOG_FILE"
                fi
                
                # catat error ke csv agar data tetap lengkap
                echo "$(date '+%Y-%m-%d %H:%M:%S'),$NAME,$count,$ERROR_MSG,$ERROR_MSG,$ERROR_MSG,$WORKERS" >> results/experiment_data.csv
            fi
            
            echo "    [Run $i/$REPETITIONS] Done."
        done
    done
done

echo -e "\n${GREEN}All experiments finished! Check results/experiment_data.csv${NC}"
