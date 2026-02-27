rm -rf testsite
go build -o pubengine ./cmd/pubengine/
./pubengine new testsite
cd testsite
npm install
make run
