FROM mcr.microsoft.com/windows/nanoserver:ltsc2022
WORKDIR /app
RUN powershell.exe -Command \
    wget -Uri https://dl.google.com/go/go1.16.3.windows-amd64.zip -OutFile c:\go.zip ; \
    Expand-Archive -Path c:\go.zip -DestinationPath c:\ ; \
    Remove-Item c:\go.zip
RUN setx path "%path%;c:\go\bin"
COPY . .
RUN go build -o main .
CMD ["./main"]