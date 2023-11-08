# gptcli
A CLI based interface to OpenAI's GPT API

## Building

```bash
make
```

## Installing

```bash
mkdir -p $HOME/bin
GPTCLI=$(curl -s https://api.github.com/repos/mikeb26/gptcli/releases/latest | grep browser_download_url | cut -f4 -d\")
wget $GPTCLI
chmod 755 gptcli
mv gptcli $HOME/bin
# add $HOME/bin to your $PATH if not already present
```
## Contributing
Pull requests are welcome at https://github.com/mikeb26/gptcli

For major changes, please open an issue first to discuss what you
would like to change.

## License
[AGPL3](https://www.gnu.org/licenses/agpl-3.0.en.html)
