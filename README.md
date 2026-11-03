# 🐳 go-lab

Servidor HTTP mínimo en **Go** (solo librería estándar), empaquetado en una imagen Docker *multi-stage* de ~16 MB y publicable en un repositorio privado de **Amazon ECR**.

## 🏗️ Arquitectura

```text
 main.go + handlers.go
          │
          ▼
 ┌─────────────────── Dockerfile (multi-stage) ───────────────────┐
 │  deps ──► test ──► build ──────────────► final                 │
 │  (go mod)  (vet +   (binario estático)   (distroless nonroot,  │
 │            tests)                         solo el binario)     │
 └────────────────────────────────────────────────────────────────┘
          │
          ├── docker run ──► http://localhost:8080
          └── docker push ──► Amazon ECR (repositorio privado)
```

Un test que falla **detiene el build**: la etapa `build` depende de `test`.

## 🌐 Endpoints

| Ruta | Método | Respuesta |
| --- | --- | --- |
| `/` | GET | JSON con nombre, versión, commit, fecha de compilación, hostname, versión de Go, entorno y uptime |
| `/healthz` | GET | `200` si el proceso está vivo |
| `/version` | GET | JSON con versión, commit y fecha de compilación |
| otra ruta | cualquiera | `404` en JSON |

Un método no permitido responde `405` con el header `Allow`.

## ⚙️ Configuración

| Variable | Por defecto | Uso |
| --- | --- | --- |
| `PORT` | `8080` | Puerto de escucha |
| `APP_ENV` | `dev` | Entorno mostrado en `/` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` o `error` |

Un valor inválido termina el proceso al arrancar con un error claro.

## 💻 Ejecución local (sin Docker)

```bash
go vet ./... && go test -race ./...   # tests
go run .                               # arranca en :8080
curl -i http://localhost:8080/
```

## 📦 Construir y ejecutar con Docker

```bash
# Construir con versión, commit y fecha inyectados
docker build \
  --build-arg VERSION=1.0.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t go-lab:1.0.0 .

# Ejecutar con privilegios mínimos
docker run -d --name go-lab -p 8080:8080 \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  go-lab:1.0.0

docker ps                              # debe mostrar "healthy"
curl -s http://localhost:8080/version
```

Solo los tests: `docker build --target test .`
Otra arquitectura: añade `--platform linux/arm64` (por defecto, la de tu equipo).

Apagado ordenado: `docker stop go-lab` tarda menos de un segundo, porque la app atiende `SIGTERM`.

## ☁️ Publicar en Amazon ECR

Sustituye `<ACCOUNT_ID>` y `<REGION>` por los de tu cuenta, y no publiques el ID real.

```bash
# 1. Autenticar Docker contra ECR (el token dura 12 horas)
aws ecr get-login-password --region <REGION> | \
  docker login --username AWS --password-stdin <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com

# 2. Etiquetar con el URI del repositorio y subir
docker tag go-lab:1.0.0 <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/go-lab:1.0.0
docker push <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/go-lab:1.0.0

# 3. Comprobar tag y digest
aws ecr describe-images --repository-name go-lab --region <REGION>
```

El repositorio es privado y usa *tag immutability*: un tag publicado no se puede sobrescribir (para corregir algo, publica `1.0.1`).

## 🔒 Decisiones de diseño

- **Go sin dependencias:** binario estático pequeño e imagen auditable.
- **Multi-stage + distroless:** la imagen final no tiene shell, compilador ni código fuente.
- **Usuario no root** y `ENTRYPOINT` en forma *exec*: el binario es el PID 1 y recibe `SIGTERM`.
- **Healthcheck sin `curl`:** `/app healthcheck` consulta `/healthz` y sale con código `0` si responde `200`.
- **Versiones fijas:** nunca `latest`.

## 🧹 Limpieza

```bash
docker rm -f go-lab
docker rmi go-lab:1.0.0
aws ecr delete-repository --repository-name go-lab --force --region <REGION>
```
