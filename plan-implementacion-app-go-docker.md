# Plan de implementación: app web en Go e imagen Docker multi-stage

> Documento de planificación para que un agente (o tú) implemente. **No contiene código** a propósito: define qué construir, cómo debe comportarse y cómo se acepta el resultado. Acompaña al taller `taller-docker-ecr-go.md`.

## 1. Objetivo

Construir un servicio HTTP pequeño en Go y una imagen Docker de producción mínima, segura y reproducible, publicada en un repositorio privado de Amazon ECR. Debe funcionar de verdad y servir como pieza de portafolio para un perfil DevOps / SRE.

## 2. Restricciones técnicas

- **Lenguaje:** Go, con la **librería estándar únicamente**. Sin frameworks ni dependencias externas. Esto mantiene el binario pequeño y la imagen auditable.
- **Versión de Go:** la que tenga instalada la persona en su equipo (`go version`). La misma versión exacta se fija en la imagen del builder. El agente debe confirmarla antes de escribir el `go.mod` y el Dockerfile; no asumir una versión.
- **Binario estático:** sin CGO, compilado para Linux.
- **Un solo proceso, un solo puerto:** el servicio escucha en `0.0.0.0` y el puerto sale de una variable de entorno.
- **Sin secretos en el repositorio:** nada de credenciales, IDs de cuenta ni URIs reales. El ID de cuenta de AWS se representa con un marcador.
- **Funciona sin Docker:** `go run` y `go test` deben bastar para desarrollar.

## 3. Estructura de archivos propuesta

**Decisión:** el proyecto vive en la **raíz de este repositorio** (`commit-lab-docker`), sin subcarpeta `go-lab/`. El módulo de Go se llama `commit-lab-docker`; el nombre de la **imagen** sigue siendo `go-lab` (como en el taller, p. ej. `go-lab:1.0.0`). La versión de Go confirmada con `go version` es **go1.26.5** y `go.mod` la fija exacta (`go 1.26.5`); el Dockerfile debe usar esa misma versión en el builder.

```text
.                           # raíz del repositorio
├── main.go                 # arranque, configuración, servidor y apagado
├── handlers.go             # manejadores, middleware y variables de versión
├── handlers_test.go        # tests con httptest
├── go.mod                  # módulo commit-lab-docker, go 1.26.5
├── .gitignore
├── Dockerfile
├── .dockerignore           # debe excluir taller.md y este plan (ver 5.4)
├── Makefile
├── scripts/
│   └── smoke.sh            # prueba de humo contra un contenedor en marcha
├── docs/
│   ├── architecture.md     # diagrama y explicación
│   └── evidence/           # capturas (sin ID de cuenta)
├── README.md
├── taller.md               # ya existe: referencia del taller (solo lectura)
└── plan-implementacion-app-go-docker.md   # ya existe: este documento
```

`taller.md` y este plan ya existen en la raíz y no forman parte de la imagen: el `.dockerignore` debe excluirlos (junto con `.atl/`).

El agente puede ajustar el reparto de archivos si lo justifica, manteniendo la separación entre arranque y manejadores para poder probarlos.

### 3.1 Valores elegidos en la implementación

| Aspecto | Valor | Motivo |
| --- | --- | --- |
| Argumento de healthcheck | `healthcheck` (`./app healthcheck`) | Simple y explícito; consulta `http://127.0.0.1:$PORT/healthz` con timeout de 2 s. El `HEALTHCHECK` del Dockerfile debe invocar `["/app", "healthcheck"]` (ajustar a la ruta real del binario) |
| Plazo de apagado | 5 s | Menor que los 10 s de Docker antes de `SIGKILL`; `Shutdown` cierra al instante las conexiones *keep-alive* inactivas, así que en reposo el apagado tarda milisegundos. Si vence el plazo se fuerza `Close` y el código de salida es `1` |
| `ReadHeaderTimeout` | 5 s | Mitiga clientes lentos (Slowloris) |
| `ReadTimeout` / `WriteTimeout` | 10 s / 10 s | Las respuestas son JSON diminutos; margen amplio |
| `IdleTimeout` | 60 s | Permite reutilizar conexiones sin retenerlas indefinidamente |
| `MaxHeaderBytes` | 16 KiB | Suficiente para uso normal; el valor por defecto de Go es 1 MiB |
| Códigos de salida | `0` ok, `1` fallo en ejecución, `2` configuración o argumento inválido | Distinguir errores de configuración de errores en ejecución |
| `Allow` en 405 | `GET` | Las tres rutas solo aceptan `GET` |

## Estado de implementación

| Elemento | Estado |
| --- | --- |
| Aplicación Go (`main.go`, `handlers.go`, `handlers_test.go`, `go.mod`, `.gitignore`) | Hecho y verificado (`gofmt`, `go vet`, `go test -race`, ejecución real, apagado, healthcheck, `ldflags`) |
| `Dockerfile` (4 etapas) | Pendiente |
| `.dockerignore` | Pendiente |
| `Makefile` | Pendiente |
| `scripts/smoke.sh` | Pendiente |
| Variantes A/B/C/D y tabla de tamaños | Pendiente |
| `README.md` | Pendiente |
| `docs/` (arquitectura y evidencias) | Pendiente |

## 4. Especificación de la aplicación

### 4.1 Rutas

| Ruta | Método | Respuesta |
| --- | --- | --- |
| `/` | GET | JSON con: nombre de la aplicación, versión, commit, fecha de compilación, hostname del contenedor, versión de Go con que se compiló, entorno (`APP_ENV`) y tiempo en marcha |
| `/healthz` | GET | `200` con un cuerpo mínimo si el proceso está vivo |
| `/version` | GET | JSON solo con versión, commit y fecha de compilación |
| Cualquier otra | cualquiera | `404` en JSON, con un mensaje claro |

Los métodos no permitidos en una ruta deben responder `405`, con el header `Allow`. Todas las respuestas JSON llevan `Content-Type` correcto.

### 4.2 Configuración por variables de entorno

| Variable | Por defecto | Uso |
| --- | --- | --- |
| `PORT` | `8080` | Puerto de escucha |
| `APP_ENV` | `dev` | Entorno mostrado en `/` |
| `LOG_LEVEL` | `info` | Nivel mínimo de log (`debug`, `info`, `warn`, `error`) |

Un valor inválido (por ejemplo un puerto no numérico) debe terminar el proceso al arrancar con un mensaje de error claro y código de salida distinto de cero.

### 4.3 Información de versión

La versión, el commit y la fecha de compilación se **inyectan al compilar** (variables sobrescritas en tiempo de enlace), no se escriben a mano. Si no se inyectan (por ejemplo con `go run`), los valores por defecto deben ser `dev`, `unknown` y `unknown`.

### 4.4 Logging

- Librería estándar (`log/slog`), formato **JSON** a la salida estándar.
- Un registro por petición, con método, ruta, código de respuesta, duración y un identificador de petición. Si la petición trae `X-Request-ID`, se reutiliza; si no, se genera uno y se devuelve en la respuesta.
- Un registro al arrancar (versión, commit, puerto) y otro al apagar.

### 4.5 Robustez del servidor

- Definir tiempos de espera en el servidor HTTP: para leer cabeceras, leer la petición completa, escribir la respuesta e inactividad. El agente elige valores razonables y los documenta.
- Limitar el tamaño de las cabeceras.
- **Apagado ordenado:** al recibir `SIGTERM` o `SIGINT`, dejar de aceptar conexiones nuevas, esperar a las peticiones en curso hasta un plazo máximo y terminar con código `0`. El plazo debe ser menor que los 10 segundos que Docker espera por defecto antes de forzar.
- El proceso debe ser el **PID 1** del contenedor (por eso el `ENTRYPOINT` va en forma *exec*).

### 4.6 Modo de healthcheck

Una imagen mínima no tiene `curl` ni shell. Para que el `HEALTHCHECK` del Dockerfile funcione, el propio binario debe poder ejecutarse en un **modo de verificación**: si se invoca con un argumento específico, hace una petición a `/healthz` en el puerto local, termina con código `0` si responde `200` y distinto de cero en cualquier otro caso. El resto del tiempo arranca el servidor normalmente.

### 4.7 Tests

Con `net/http/httptest`, sin red real. Cubrir como mínimo:

- `/`, `/healthz` y `/version` devuelven `200` y el JSON esperado.
- Una ruta inexistente devuelve `404`.
- Un método no permitido devuelve `405` con `Allow`.
- El identificador de petición se genera y se reutiliza.
- La carga de configuración aplica valores por defecto y rechaza valores inválidos.

## 5. Especificación del Dockerfile

### 5.1 Etapas

| Etapa | Base | Responsabilidad |
| --- | --- | --- |
| `deps` | Imagen oficial de Go, **versión exacta fija** | Copiar `go.mod` (y `go.sum` si existe) y descargar dependencias. Va antes de copiar el código para aprovechar la caché de capas |
| `test` | Hereda de `deps` | Copiar el código y ejecutar `go vet` y `go test`. Debe poder construirse sola con `--target test` |
| `build` | Hereda de `deps` | Compilar un binario estático para Linux con estas condiciones: CGO desactivado, `-trimpath`, símbolos de depuración recortados, y versión, commit y fecha inyectados desde argumentos de construcción. La salida es un único binario |
| `final` | Distroless estática, variante *nonroot* | Copiar solo el binario desde `build`. Sin shell, sin gestor de paquetes |

La etapa `final` **debe depender de `test`** para que un test fallido impida producir la imagen (por ejemplo, copiando un artefacto desde la etapa `test` o forzando el orden de otra manera). El agente debe elegir un mecanismo que funcione tanto con BuildKit como con `--target test` y documentarlo.

### 5.2 Requisitos de la imagen final

- Se ejecuta como **usuario no root**.
- Declara el puerto `8080` con `EXPOSE`.
- `ENTRYPOINT` en **forma *exec*** (lista), sin shell.
- Incluye `HEALTHCHECK` que invoque el modo de verificación del binario (sección 4.6), con intervalo, tiempo de espera y reintentos razonables.
- Metadatos con las etiquetas estándar OCI: título, descripción, versión, commit, fecha de creación y URL del código fuente. Los valores salen de los argumentos de construcción.
- Argumentos de construcción: `VERSION`, `COMMIT`, `BUILD_DATE`, con valores por defecto neutros.

### 5.3 Reproducibilidad y caché

- Fijar la versión de las imágenes base; nunca `latest`.
- Ordenar las instrucciones para maximizar el uso de la caché: dependencias primero, código después.
- Opcional: usar montajes de caché de BuildKit para los módulos y la caché de compilación de Go. Debe seguir funcionando sin ellos.
- La plataforma de destino por defecto es `linux/amd64`; documentar cómo construir otra con `--platform`.

### 5.4 `.dockerignore`

Excluir al menos: `.git`, binarios compilados, `docs/`, `Makefile`, archivos de entorno y editores, y el propio `Dockerfile` si no es necesario en el contexto.

## 6. Makefile

Objetivos mínimos, cada uno con una descripción corta (`make help`):

| Objetivo | Acción |
| --- | --- |
| `test` | Ejecutar `go vet` y `go test` en local |
| `run` | Ejecutar la app en local |
| `build` | Construir la imagen con versión, commit y fecha inyectados |
| `up` / `down` | Arrancar y detener el contenedor local |
| `smoke` | Ejecutar `scripts/smoke.sh` |
| `sizes` | Construir las 4 variantes del taller y mostrar una tabla de tamaños |
| `ecr-login` | Autenticar Docker contra ECR |
| `push` | Etiquetar y subir la imagen a ECR |

Región, ID de cuenta, nombre del repositorio y versión salen de **variables**, sin valores reales dentro del archivo.

## 7. Variantes para el experimento de tamaño

Para la tabla comparativa del taller, el repositorio debe poder generar:

| Variante | Descripción |
| --- | --- |
| A | Una sola etapa sobre la imagen oficial de Go |
| B | Multi-stage con base final Alpine |
| C | Multi-stage con base final distroless *nonroot* (la variante principal) |
| D | Multi-stage con base final `scratch` |

Pueden vivir como Dockerfiles alternativos en una carpeta `docs/variants/` o como un único Dockerfile con una etapa final seleccionable. El agente decide y lo documenta. La variante C es la oficial del proyecto. La D debe declarar en un comentario las limitaciones de `scratch` (sin certificados de CA, sin zona horaria, sin `/etc/passwd`).

## 8. Script de humo

`scripts/smoke.sh` ejecuta contra una URL base (por defecto `http://localhost:8080`) y falla con código distinto de cero si algo no se cumple:

1. `/healthz` responde `200`.
2. `/version` devuelve JSON con versión y commit no vacíos.
3. `/` devuelve JSON con los campos esperados.
4. Una ruta inexistente responde `404`.
5. Muestra un resumen de aciertos y fallos.

## 9. README del repositorio

En este orden: qué es el proyecto, diagrama del flujo, decisiones de diseño (por qué Go sin dependencias, multi-stage, distroless, usuario no root, tags inmutables, `ENTRYPOINT` en forma *exec*), cómo desarrollar en local, cómo construir y ejecutar, cómo publicar en ECR (con `<ACCOUNT_ID>` y `<REGION>` como marcadores), resultados medidos (tabla de tamaños, escaneo, prueba de apagado), limitaciones, mejoras futuras y limpieza. Tono técnico y directo, extensión moderada, sin capturas con IDs de cuenta.

## 10. Criterios de aceptación

**Aplicación**

- [ ] `go vet` y `go test` pasan sin errores.
- [ ] `go run` arranca en el puerto 8080 y las tres rutas responden según la sección 4.1.
- [ ] Un puerto inválido en `PORT` termina el proceso con error claro.
- [ ] Los logs salen en JSON, con un registro por petición.
- [ ] `SIGTERM` provoca un apagado ordenado con código de salida `0`.
- [ ] El modo de healthcheck responde con código `0` cuando la app está sana y distinto de cero si no.

**Imagen**

- [ ] `docker build` termina sin errores y `docker build --target test` ejecuta solo los tests.
- [ ] Un test que falle impide generar la imagen final.
- [ ] La imagen final no contiene shell, compilador ni código fuente.
- [ ] El contenedor corre como usuario no root (verificable con `docker inspect`).
- [ ] La aplicación escucha en `0.0.0.0` y responde desde el equipo anfitrión con `-p 8080:8080`.
- [ ] Funciona con `--read-only --cap-drop ALL --security-opt no-new-privileges`.
- [ ] `docker ps` muestra el contenedor como `healthy`.
- [ ] `time docker stop` tarda menos de 2 segundos.
- [ ] `/version` muestra el commit inyectado y coincide con `git rev-parse --short HEAD`.
- [ ] Tamaño de la imagen final: **objetivo** de pocos MB, claramente menor que la variante A. El agente reporta los valores reales de las 4 variantes.

**Entrega**

- [ ] Estructura de carpetas según la sección 3.
- [ ] README completo según la sección 9.
- [ ] Sin secretos, credenciales ni IDs de cuenta en el repositorio.
- [ ] `make smoke` pasa contra el contenedor en marcha.

## 11. Verificación manual sugerida

1. Ejecutar los tests y la app sin Docker.
2. Construir la imagen y ejecutar el contenedor.
3. Pasar el script de humo.
4. Comprobar `docker ps`, `docker inspect` (usuario y estado de salud) y `time docker stop`.
5. Probar con `--read-only --cap-drop ALL --security-opt no-new-privileges`.
6. Romper un test a propósito y verificar que el build se detiene; revertirlo.
7. Construir las 4 variantes y anotar los tamaños.
8. Publicar en ECR siguiendo el taller y hacer la prueba de extremo a extremo (borrar local, bajar, ejecutar).

## 12. Fuera de alcance

- Frameworks web, ORMs, bases de datos y cualquier dependencia externa.
- Autenticación, HTTPS y balanceo dentro de la aplicación.
- Orquestación (ECS, Kubernetes) y pipelines de CI/CD; quedan como mejoras posteriores.
- Cualquier cambio en los requisitos y entregables oficiales de la academia.

## 13. Resumen de instrucciones para el agente

Confirma primero la versión de Go instalada. Implementa la estructura de la sección 3. Escribe la aplicación según la sección 4, con solo la librería estándar y con sus tests. Escribe el Dockerfile de la sección 5 con sus cuatro etapas, el `.dockerignore`, el Makefile y el script de humo. Redacta el README de la sección 9. Valida cada criterio de la sección 10 y reporta los resultados reales, incluidos los tamaños. Si algún punto de este plan es ambiguo o contradictorio con lo que observes, **pregunta antes de improvisar**.
