server {
    listen {{.NginxPort}};
    server_name localhost;

    {{range .Consoles}}
    {{- if (eq .Status "connected")}}
    location /{{.NginxUuid}}/ {
        auth_basic "Restricted";
        auth_basic_user_file /app/loomis/config/{{.UdevId}}.htpass;
        
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://127.0.0.1:{{.ShellPort}};
    }
    {{end -}}
    {{end}}
}
