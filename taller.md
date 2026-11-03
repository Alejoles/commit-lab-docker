# Taller: app web en Go, imagen Docker y publicación en Amazon ECR

> El enunciado, los requisitos y los entregables son los originales de la academia. Todo lo que añadí va marcado con **(extra)**. En los comandos, `<ACCOUNT_ID>` y `<REGION>` son marcadores: sustitúyelos por los de tu cuenta y no publiques el ID real.

## Enunciado

En este laboratorio deberás crear una aplicación web sencilla en Python (u otro lenguaje), contenerizarla utilizando Docker y publicar la imagen resultante en un repositorio privado de Amazon Elastic Container Registry dentro de tu cuenta de Amazon Web Services.

El objetivo es aplicar los conocimientos vistos en el curso sobre:

- creación de aplicaciones simples
- construcción de imágenes Docker mediante un Dockerfile
- ejecución de contenedores Docker
- gestión de imágenes en un registro de contenedores

La aplicación debe ejecutarse como un servidor web y responder a peticiones HTTP.

**(extra) Lenguaje:** se usa **Go** ("otro lenguaje" según el enunciado). Compila a un binario estático, lo que permite una imagen final muy pequeña con un Dockerfile *multi-stage*.

## 📋 Requisitos del laboratorio

- Crear una aplicación web sencilla que responda a peticiones HTTP.
- Crear un Dockerfile que permita ejecutar esa aplicación dentro de un contenedor.
- Construir una imagen Docker a partir del Dockerfile.
- Ejecutar el contenedor en local y comprobar que funciona correctamente.
- Verificar el funcionamiento de la aplicación mediante una petición HTTP desde la terminal.
- Crear un repositorio privado en Amazon ECR.
- Subir la imagen Docker al repositorio.

## Arquitectura (extra)

```text
Código Go ──► Dockerfile multi-stage ──► Imagen mínima ──► docker run + curl (local)
                                              │
                                              └── docker push ──► ECR (repositorio privado)

```

---

## Paso 1 – Crear la aplicación web

Servidor HTTP en Go con la librería estándar (`net/http`), escuchando en el puerto `8080`.

**(extra)** Rutas y comportamiento:


| Ruta       | Respuesta                                                                    |
| ---------- | ---------------------------------------------------------------------------- |
| `/`        | JSON con nombre, versión, commit, hostname del contenedor y tiempo en marcha |
| `/healthz` | `200 OK` si el proceso está vivo                                             |
| `/version` | Versión, commit y fecha de compilación                                       |


- Escucha en `0.0.0.0`, no en `localhost`: dentro de un contenedor, `localhost` no es accesible desde fuera.
- Puerto configurable con la variable `PORT` (por defecto 8080).
- Logs en JSON y apagado ordenado al recibir `SIGTERM`.
- Tests con `net/http/httptest` (`go test ./...`).

## Paso 2 – Crear el Dockerfile

Dockerfile *multi-stage*: una etapa compila con el SDK de Go y la imagen final solo contiene el binario.

**(extra)** Etapas recomendadas:


| Etapa   | Base                                | Función                                                                       |
| ------- | ----------------------------------- | ----------------------------------------------------------------------------- |
| `test`  | Imagen oficial de Go (versión fija) | Ejecuta `go vet` y `go test`                                                  |
| `build` | Imagen oficial de Go (versión fija) | Compila un binario estático (`CGO_ENABLED=0`) con versión y commit inyectados |
| `final` | Distroless estática (`nonroot`)     | Copia solo el binario y lo ejecuta sin privilegios                            |


- Usuario **no root**.
- `ENTRYPOINT` en forma *exec*, para que el binario reciba `SIGTERM` de `docker stop`.
- Versiones de imagen fijas (nunca `latest`) y un `.dockerignore`.

## Paso 3 – Construir la imagen

```bash
docker build -t go-lab:1.0.0 .

```

**(extra)** Inyectar versión y commit:

```bash
docker build \
  --build-arg VERSION=1.0.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  -t go-lab:1.0.0 .

```

**(extra)** Si construyes en un Mac con chip Apple y vas a ejecutar en x86, añade `--platform linux/amd64`; si no, la imagen falla con `exec format error`.

**(extra)** Compara tamaños (`docker images`) y completa la tabla con tus valores:


| Variante                              | Tamaño |
| ------------------------------------- | ------ |
| Una sola etapa (imagen oficial de Go) |        |
| Multi-stage + distroless              |        |


## Paso 4 – Ejecutar el contenedor en local

```bash
docker run -d --name go-lab -p 8080:8080 go-lab:1.0.0
docker ps

```

**(extra)** Ejecución con privilegios mínimos (la app debe seguir funcionando):

```bash
docker run -d --name go-lab -p 8080:8080 \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  go-lab:1.0.0

```

**(extra)** Comprueba que el apagado es ordenado: `time docker stop go-lab` debe tardar un par de segundos como máximo. Si tarda unos 10 s, la app ignora `SIGTERM`.

## Paso 5 – Verificar con una petición HTTP

```bash
curl -i http://localhost:8080/

```

**(extra)** Verifica los demás endpoints y que el commit coincide con el de Git:

```bash
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/version

```

## Paso 6 – Crear el repositorio privado en ECR

En la consola: ECR → *Repositories* → *Create repository* → visibilidad **Private** → nombre `go-lab`.

**(extra)** Opciones recomendadas:

- **Tag immutability** activada: un tag publicado no se puede sobrescribir (para corregir algo, publica `1.0.1`).
- **Scan on push** activado: escaneo de vulnerabilidades al subir.
- Política de ciclo de vida que expire las imágenes sin tag.

Si tu rol de laboratorio bloquea alguna de estas opciones, anótalo como limitación del entorno.

## Paso 7 – Subir la imagen

```bash
# Autenticar Docker contra ECR (el token dura 12 horas)
aws ecr get-login-password --region <REGION> | \
  docker login --username AWS --password-stdin <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com

# Etiquetar con el URI del repositorio y subir
docker tag go-lab:1.0.0 <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/go-lab:1.0.0
docker push <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/go-lab:1.0.0

```

El URI de la imagen es `<ACCOUNT_ID>.dkr.ecr.<REGION>.[amazonaws.com/go-lab:1.0.0](http://amazonaws.com/go-lab:1.0.0)`.

**(extra)** Anota el *digest* (`sha256:...`) con `aws ecr describe-images --repository-name go-lab --region <REGION>`: es la identidad inmutable de la imagen; el tag es solo una etiqueta.

**(extra)** Prueba de extremo a extremo: borra la imagen local, bájala de ECR y ejecútala.

```bash
docker rm -f go-lab
docker rmi go-lab:1.0.0 <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/go-lab:1.0.0
docker pull <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/go-lab:1.0.0
docker run -d --name go-lab -p 8080:8080 <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/go-lab:1.0.0
curl -s http://localhost:8080/version

```

---

## 📦 Entregable

El estudiante deberá entregar, en un fichero comprimido:

1. El/los archivos con la aplicación.
2. El archivo `Dockerfile` utilizado para construir la imagen.
3. Captura del contenedor ejecutándose en local.
4. Captura de una petición `curl` desde local mostrando la respuesta del servidor.
5. El URI completo de la imagen publicada en ECR.
6. Una breve explicación (8–10 líneas) describiendo:
   - qué hace la aplicación
   - qué función cumple el Dockerfile y cómo lo ha construido
   - qué es una imagen Docker
   - para qué se utiliza Amazon ECR

**(extra)** Anexo opcional en la entrega: tabla de tamaños, captura de ECR (tag, digest y tamaño), resultado del escaneo y la prueba de extremo a extremo.

## Costos y limpieza (extra)

- **Costos:** ECR cobra principalmente por GB-mes almacenado y por transferencia de salida. Consulta los precios y niveles gratuitos vigentes en la página oficial de ECR; una imagen mínima reduce almacenamiento y tiempos de `push` y `pull`.
- **Limpieza:** `docker rm -f go-lab`, elimina las imágenes locales y borra el repositorio de ECR (consola o `aws ecr delete-repository --repository-name go-lab --force --region <REGION>`).