rm -rf testsite
go build -o pubengine ./cmd/pubengine/
./pubengine new testsite
cd testsite
npm install
cp .env.example .env
go mod tidy
make run
