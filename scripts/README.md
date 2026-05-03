# Installation Scripts

Simple scripts to download go-fim binary to your current directory.

## `install.sh` (Linux / macOS)

```bash
# Latest version
curl -sSL https://raw.githubusercontent.com/Jimzical/go-fim/main/scripts/install.sh | bash

# Specific version
curl -sSL https://raw.githubusercontent.com/Jimzical/go-fim/main/scripts/install.sh | bash -s 1.0.0
```

This downloads `go-fim` to your current directory. To install system-wide:
```bash
sudo mv go-fim /usr/local/bin/
```

---

## `install.ps1` (Windows)

```powershell
# Latest version
iwr -useb https://raw.githubusercontent.com/Jimzical/go-fim/main/scripts/install.ps1 | iex

# Specific version (download script first)
iwr -useb https://raw.githubusercontent.com/Jimzical/go-fim/main/scripts/install.ps1 -OutFile install.ps1
.\install.ps1 -Version "1.0.0"
```

This downloads `go-fim.exe` to your current directory. To install system-wide:
```powershell
move go-fim.exe C:\Windows\System32\
```

---

## Testing Locally

Before pushing changes, test the scripts locally:

**Linux / macOS:**
```bash
cd scripts
chmod +x install.sh
./install.sh 1.0.0
```

**Windows:**
```powershell
cd scripts
.\install.ps1 -Version "1.0.0"
```

## Troubleshooting

**Download fails:** Check that the version exists at https://github.com/Jimzical/go-fim/releases

**Windows execution policy error:**
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```
