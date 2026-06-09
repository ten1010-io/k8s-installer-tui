pipeline {
    agent any

    tools {
        go 'go-1.24.3'
    }

    environment {
        BINARY   = 'k8s-installer-tui'
        DIST_DIR = 'dist'
    }

    options {
        buildDiscarder(logRotator(numToKeepStr: '10'))
        timeout(time: 20, unit: 'MINUTES')
        disableConcurrentBuilds()
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Tag') {
            // main 브랜치 push 시 VERSION 파일 기준으로 자동 태깅
            // GIT_BRANCH는 일반 파이프라인에서 "origin/main" 형태로 설정됨
            when {
                expression { return env.GIT_BRANCH ==~ /.*\/main|^main$/ }
                not { buildingTag() }
            }
            steps {
                withCredentials([usernamePassword(credentialsId: 'ten-infra', usernameVariable: 'GH_USER', passwordVariable: 'GH_TOKEN')]) {
                    sh '''
                        VERSION=v$(cat VERSION)
                        REMOTE_URL=https://x-access-token:${GH_TOKEN}@$(git remote get-url origin | sed 's|https://||')
                        if git ls-remote --tags "${REMOTE_URL}" | grep -q "refs/tags/${VERSION}$"; then
                            echo "Tag ${VERSION} already exists, skipping tag creation."
                        else
                            git config user.email "jenkins@ci"
                            git config user.name "Jenkins"
                            git tag -a "${VERSION}" -m "Release ${VERSION}"
                            git push "${REMOTE_URL}" "refs/tags/${VERSION}"
                        fi
                        echo "${VERSION}" > tag.env
                    '''
                    script {
                        env.TAG_NAME = readFile('tag.env').trim()
                    }
                }
            }
        }

        stage('Deps') {
            steps {
                sh 'go mod tidy'
                sh 'go mod verify'
            }
        }

        stage('Vet') {
            steps {
                sh 'go vet ./...'
            }
        }

        stage('Build') {
            steps {
                sh '''
                    VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
                    LDFLAGS="-X main.version=${VERSION} -s -w"
                    mkdir -p dist
                    GOOS=linux GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o dist/${BINARY}-linux-amd64 .
                    GOOS=linux GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o dist/${BINARY}-linux-arm64 .
                '''
            }
        }

        stage('Archive') {
            steps {
                archiveArtifacts artifacts: "${DIST_DIR}/${BINARY}-linux-*",
                                 fingerprint: true
            }
        }

        stage('Release') {
            when {
                expression { return env.TAG_NAME?.startsWith('v') }
            }
            steps {
                withCredentials([usernamePassword(credentialsId: 'ten-infra', usernameVariable: 'GH_USER', passwordVariable: 'GH_TOKEN')]) {
                    sh '''
                        REPO=$(git remote get-url origin | sed 's|https://github.com/||;s|\\.git$||')

                        STATUS=$(curl -sL -o release.json -w "%{http_code}" \
                            -H "Authorization: Bearer ${GH_TOKEN}" \
                            "https://api.github.com/repos/${REPO}/releases/tags/${TAG_NAME}")

                        if [ "${STATUS}" != "200" ]; then
                            echo "Creating release ${TAG_NAME}..."
                            curl -fsSL -X POST \
                                -H "Authorization: Bearer ${GH_TOKEN}" \
                                -H "Content-Type: application/json" \
                                "https://api.github.com/repos/${REPO}/releases" \
                                -d "{\\"tag_name\\":\\"${TAG_NAME}\\",\\"name\\":\\"${TAG_NAME}\\",\\"body\\":\\"Release ${TAG_NAME}\\"}" \
                                -o release.json
                        else
                            echo "Release ${TAG_NAME} already exists."
                        fi
                    '''
                    script {
                        def release = new groovy.json.JsonSlurper().parseText(readFile('release.json'))
                        env.UPLOAD_URL = release.upload_url.replaceAll(/\{[^}]*\}/, '')
                    }
                    sh '''
                        for FILE in dist/${BINARY}-linux-amd64 dist/${BINARY}-linux-arm64; do
                            curl -fsSL -X POST \
                                -H "Authorization: Bearer ${GH_TOKEN}" \
                                -H "Content-Type: application/octet-stream" \
                                "${UPLOAD_URL}?name=$(basename ${FILE})" \
                                --data-binary @${FILE}
                        done
                    '''
                }
            }
        }
    }

    post {
        always {
            sh "rm -rf ${DIST_DIR} tag.env release.json 2>/dev/null || true"
        }
        success {
            echo "Build ${env.BUILD_NUMBER} succeeded — ${env.GIT_COMMIT?.take(7)}"
        }
        failure {
            echo "Build ${env.BUILD_NUMBER} failed"
        }
    }
}

