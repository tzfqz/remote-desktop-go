import os

files = {
    r'D:\appai\remote-desktop-go\server\main.go': 'server_config.yaml',
    r'D:\appai\remote-desktop-go\client\agent\main.go': 'agent_config.yaml',
    r'D:\appai\remote-desktop-go\client\controller\main.go': 'controller_config.yaml',
}

for fp, new_name in files.items():
    with open(fp, 'r', encoding='utf-8') as f:
        c = f.read()
    c = c.replace('config.Load("config.yaml")', f'config.Load("{new_name}")')
    with open(fp, 'w', encoding='utf-8') as f:
        f.write(c)
    print(f'updated: {fp} -> {new_name}')
