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
            parallel {
                stage('linux/amd64') {
                    steps {
                        sh 'make build-linux'
                    }
                }
                stage('linux/arm64') {
                    steps {
                        sh 'make build-linux-arm64'
                    }
                }
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
