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
    }

    post {
        always {
            sh "rm -rf ${DIST_DIR}"
        }
        success {
            echo "Build ${env.BUILD_NUMBER} succeeded — ${env.GIT_COMMIT?.take(7)}"
        }
        failure {
            echo "Build ${env.BUILD_NUMBER} failed"
        }
    }
}
