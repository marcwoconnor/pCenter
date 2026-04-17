# APT Repository Setup

One-time setup to enable `apt install pcenter`.

## 1. Generate a GPG signing key

On your dev machine:

```bash
gpg --full-generate-key
# Select: RSA and RSA, 4096 bits, no expiry
# Name: pCenter Release Signing
# Email: marc@techsnet.net
```

Get the key ID:
```bash
gpg --list-keys --keyid-format long
# Look for the 16-char hex ID after "rsa4096/"
# Example: AB12CD34EF567890
```

Export the private key:
```bash
gpg --armor --export-secret-keys YOUR_KEY_ID > pcenter-release.gpg.private
```

## 2. Add GitHub repository secrets

Go to: github.com/marcwoconnor/pCenter/settings/secrets/actions

Add these secrets:

| Secret Name | Value |
|------------|-------|
| `APT_GPG_PRIVATE_KEY` | Contents of `pcenter-release.gpg.private` |
| `APT_GPG_KEY_ID` | Your 16-char key ID (e.g., `AB12CD34EF567890`) |

## 3. Enable GitHub Pages

Go to: github.com/marcwoconnor/pCenter/settings/pages

- Source: **Deploy from a branch**
- Branch: **gh-pages** / root
- Click Save

## 4. Create a release

```bash
git tag v0.1.0
git push origin v0.1.0
```

This triggers the GitHub Actions workflow which:
1. Builds the Go backend and React frontend
2. Packages everything into a `.deb`
3. Uploads the `.deb` to the GitHub Release
4. Updates the APT repository on GitHub Pages

## 5. Verify

After the workflow completes (~3-5 minutes):

```bash
# On any Ubuntu machine:
curl -fsSL https://marcwoconnor.github.io/pCenter/pcenter.gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/pcenter.gpg
echo "deb [signed-by=/usr/share/keyrings/pcenter.gpg] https://marcwoconnor.github.io/pCenter stable main" | sudo tee /etc/apt/sources.list.d/pcenter.list
sudo apt update
sudo apt install pcenter
```
