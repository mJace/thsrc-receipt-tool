package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func main() {
	// 1. 定義並解析命令列參數
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "台灣高鐵 PDF 收據自動解密工具\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "此工具會自動產生過去特定天數內的日期 (YYYYMMDD) 作為密碼，嘗試解密並移除 PDF 密碼保護。\n\n")
		fmt.Fprintf(os.Stderr, "可用參數:\n")
		fmt.Fprintf(os.Stderr, "  --input-folder   輸入 PDF 檔案的資料夾路徑 (預設: \"./input_pdfs\")\n")
		fmt.Fprintf(os.Stderr, "  --date-range     檢查過去的天數範圍 (預設: \"60d\")\n")
		fmt.Fprintf(os.Stderr, "  -h, --help       顯示此幫助訊息\n")
		fmt.Fprintf(os.Stderr, "\n範例:\n")
		fmt.Fprintf(os.Stderr, "  %s --input-folder ./my_receipts --date-range 30d\n", os.Args[0])
	}

	inputFolderFlag := flag.String("input-folder", "./input_pdfs", "輸入 PDF 檔案的資料夾路徑")
	dateRangeFlag := flag.String("date-range", "60d", "檢查過去的天數範圍 (例如: 60d)")
	flag.Parse()

	inputDir := *inputFolderFlag
	outputDir := "./decrypted"

	// 檢查輸入資料夾是否存在
	inputInfo, err := os.Stat(inputDir)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "錯誤: 輸入資料夾 '%s' 不存在\n", inputDir)
		os.Exit(1)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "錯誤: 無法讀取輸入資料夾: %v\n", err)
		os.Exit(1)
	}
	if !inputInfo.IsDir() {
		fmt.Fprintf(os.Stderr, "錯誤: '%s' 不是一個有效資料夾\n", inputDir)
		os.Exit(1)
	}

	// 解析日期範圍字串 (將 "60d" 轉換為整數 60)
	daysRange := 60
	trimmedRange := strings.TrimSuffix(*dateRangeFlag, "d")
	if val, err := strconv.Atoi(trimmedRange); err == nil {
		daysRange = val
	}

	// 確保輸出資料夾存在
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		os.MkdirAll(outputDir, 0755)
	}

	// 2. 產生密碼清單 (YYYYMMDD)
	passwords := generateDatePasswords(daysRange)

	// 3. 遍歷資料夾
	err = filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只處理 .pdf 檔案且跳過資料夾
		if info.IsDir() || filepath.Ext(path) != ".pdf" {
			return nil
		}

		fmt.Printf("正在處理檔案: %s\n", info.Name())

		// 4. 嘗試每個密碼 (使用並行處理，每次同時嘗試 4 個日期)
		var successPwd string
		var mu sync.Mutex
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var wg sync.WaitGroup
		sem := make(chan struct{}, 4) // 限制併發數為 4

		for _, pwd := range passwords {
			select {
			case <-ctx.Done(): // 如果已經找到密碼，停止派發新任務
				break
			default:
			}

			wg.Add(1)
			sem <- struct{}{} // 佔用一個配額
			go func(p string) {
				defer wg.Done()
				defer func() { <-sem }() // 釋放配額

				// 再次檢查是否已經成功
				select {
				case <-ctx.Done():
					return
				default:
				}

				if tryDecryptAndSave(path, outputDir, p) {
					mu.Lock()
					if successPwd == "" {
						successPwd = p
						cancel() // 通知其他 goroutine 停止
					}
					mu.Unlock()
				}
			}(pwd)
		}

		wg.Wait()
		if successPwd != "" {
			fmt.Printf("成功解密！檔案: %s, 使用密碼: %s\n", info.Name(), successPwd)
		}

		return nil
	})

	if err != nil {
		fmt.Printf("處理過程中發生錯誤: %v\n", err)
	} else {
		fmt.Println("所有任務執行完畢。")
	}
}

// generateDatePasswords 產生從今天起往回推 N 天的日期字串
func generateDatePasswords(days int) []string {
	var pwds []string
	now := time.Now()
	for i := 0; i <= days; i++ {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		pwds = append(pwds, dateStr)
	}
	return pwds
}

// tryDecryptAndSave 嘗試解密並另存新檔
func tryDecryptAndSave(srcPath, outDir, password string) bool {
	// 設定 pdfcpu 配置
	conf := model.NewDefaultConfiguration()
	conf.UserPW = password
	conf.OwnerPW = password

	// 產生目標檔名
	destBaseName := password
	destPath := generateUniqueFilename(outDir, destBaseName)

	// 使用 DecryptFile 嘗試解密，這會移除 PDF 的密碼保護並另存為無密碼的檔案
	err := api.DecryptFile(srcPath, destPath, conf)
	if err == nil {
		return true
	}

	// 如果失敗，通常是密碼不正確，刪除可能產生的錯誤碎片檔案（如果有）
	os.Remove(destPath)
	return false
}

// generateUniqueFilename 檢查檔名是否重複，若重複則加上後綴
func generateUniqueFilename(dir, baseName string) string {
	ext := ".pdf"
	finalPath := filepath.Join(dir, baseName+ext)

	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		return finalPath
	}

	// 如果檔案已存在，加上 _1, _2 ...
	counter := 1
	for {
		newName := fmt.Sprintf("%s_%d%s", baseName, counter, ext)
		finalPath = filepath.Join(dir, newName)
		if _, err := os.Stat(finalPath); os.IsNotExist(err) {
			break
		}
		counter++
	}
	return finalPath
}
